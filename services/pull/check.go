// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	git_model "code.gitea.io/gitea/models/git"
	issues_model "code.gitea.io/gitea/models/issues"
	access_model "code.gitea.io/gitea/models/perm/access"
	"code.gitea.io/gitea/models/pull"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/globallock"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/process"
	"code.gitea.io/gitea/modules/queue"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	asymkey_service "code.gitea.io/gitea/services/asymkey"
	"code.gitea.io/gitea/services/automergequeue"
	notify_service "code.gitea.io/gitea/services/notify"
)

// prPatchCheckerQueue represents a queue to handle update pull request tests
var prPatchCheckerQueue *queue.WorkerPoolQueue[string]

var (
	ErrIsClosed            = errors.New("pull is closed")
	ErrNoPermissionToMerge = errors.New("no permission to merge")
	ErrNotReadyToMerge     = errors.New("not ready to merge")
	ErrHasMerged           = errors.New("has already been merged")
	ErrIsWorkInProgress    = errors.New("work in progress PRs cannot be merged")
	ErrIsChecking          = errors.New("cannot merge while conflict checking is in progress")
	ErrNotMergeableState   = errors.New("not in mergeable state")
	ErrDependenciesLeft    = errors.New("is blocked by an open dependency")
)

func markPullRequestStatusAsChecking(ctx context.Context, pr *issues_model.PullRequest) bool {
	pr.Status = issues_model.PullRequestStatusChecking
	err := pr.UpdateColsIfNotMerged(ctx, "status")
	if err != nil {
		log.Error("UpdateColsIfNotMerged failed, pr: %-v, err: %v", pr, err)
		return false
	}
	pr, err = issues_model.GetPullRequestByID(ctx, pr.ID)
	if err != nil {
		log.Error("GetPullRequestByID failed, pr: %-v, err: %v", pr, err)
		return false
	}
	return pr.Status == issues_model.PullRequestStatusChecking
}

var AddPullRequestToCheckQueue = realAddPullRequestToCheckQueue

func realAddPullRequestToCheckQueue(prID int64) {
	err := prPatchCheckerQueue.Push(strconv.FormatInt(prID, 10))
	if err != nil && !errors.Is(err, queue.ErrAlreadyInQueue) {
		log.Error("Error adding %v to the pull requests check queue: %v", prID, err)
	}
}

func StartPullRequestCheckImmediately(ctx context.Context, pr *issues_model.PullRequest) {
	if !markPullRequestStatusAsChecking(ctx, pr) {
		return
	}
	AddPullRequestToCheckQueue(pr.ID)
}

// StartPullRequestCheckDelayable will delay the check if the pull request was not updated recently.
// When the "base" branch gets updated, all PRs targeting that "base" branch need to re-check whether
// they are mergeable.
// When there are too many stale PRs, each "base" branch update will consume a lot of system resources.
// So we can delay the checks for PRs that were not updated recently, only mark their status as
// "checking", and then next time when these PRs are updated or viewed, the real checks will run.
func StartPullRequestCheckDelayable(ctx context.Context, pr *issues_model.PullRequest) {
	if !markPullRequestStatusAsChecking(ctx, pr) {
		return
	}

	if setting.Repository.PullRequest.DelayCheckForInactiveDays >= 0 {
		if err := pr.LoadIssue(ctx); err != nil {
			return
		}
		duration := 24 * time.Hour * time.Duration(setting.Repository.PullRequest.DelayCheckForInactiveDays)
		if pr.Issue.UpdatedUnix.AddDuration(duration) <= timeutil.TimeStampNow() {
			return
		}
	}

	AddPullRequestToCheckQueue(pr.ID)
}

func StartPullRequestCheckOnView(ctx context.Context, pr *issues_model.PullRequest) {
	// TODO: its correctness totally depends on the "unique queue" feature and the global lock.
	// So duplicate "start" requests will be ignored if there is already a task in the queue or one is running.
	// Ideally in the future we should decouple the "unique queue" feature from the "start" request.
	if pr.Status == issues_model.PullRequestStatusChecking {
		if setting.IsInTesting {
			// In testing mode, there might be an "immediate" queue, which is not a real queue, everything is executed in the same goroutine
			// So we can't use the global lock here, otherwise it will cause a deadlock.
			AddPullRequestToCheckQueue(pr.ID)
		} else {
			// When a PR check starts, the task is popped from the queue and the task handler acquires the global lock
			// So we need to acquire the global lock here to prevent from duplicate tasks
			_, _ = globallock.TryLockAndDo(ctx, getPullWorkingLockKey(pr.ID), func(ctx context.Context) error {
				AddPullRequestToCheckQueue(pr.ID) // the queue is a unique queue and won't add the same task again
				return nil
			})
		}
	}
}

type MergeCheckType int

const (
	MergeCheckTypeGeneral  MergeCheckType = iota // general merge checks for "merge", "rebase", "squash", etc
	MergeCheckTypeManually                       // Manually Merged button (mark a PR as merged manually)
	MergeCheckTypeAuto                           // Auto Merge (Scheduled Merge) After Checks Succeed
)

// CheckPullMergeable check if the pull mergeable based on all conditions (branch protection, merge options, ...)
func CheckPullMergeable(stdCtx context.Context, doer *user_model.User, perm *access_model.Permission, pr *issues_model.PullRequest, mergeCheckType MergeCheckType, adminForceMerge bool) error {
	return db.WithTx(stdCtx, func(ctx context.Context) error {
		if pr.HasMerged {
			return ErrHasMerged
		}

		if err := pr.LoadIssue(ctx); err != nil {
			log.Error("Unable to load issue[%d] for %-v: %v", pr.IssueID, pr, err)
			return err
		} else if pr.Issue.IsClosed {
			return ErrIsClosed
		}

		if allowedMerge, err := IsUserAllowedToMerge(ctx, pr, *perm, doer); err != nil {
			log.Error("Error whilst checking if %-v is allowed to merge %-v: %v", doer, pr, err)
			return err
		} else if !allowedMerge {
			return ErrNoPermissionToMerge
		}

		if mergeCheckType == MergeCheckTypeManually {
			// if doer is doing "manually merge" (mark as merged manually), do not check anything
			return nil
		}

		if pr.IsWorkInProgress(ctx) {
			return ErrIsWorkInProgress
		}

		if !pr.CanAutoMerge() && !pr.IsEmpty() {
			return ErrNotMergeableState
		}

		if pr.IsChecking() {
			return ErrIsChecking
		}

		if err := CheckPullBranchProtections(ctx, pr, false); err != nil {
			if !errors.Is(err, ErrNotReadyToMerge) {
				log.Error("Error whilst checking pull branch protection for %-v: %v", pr, err)
				return err
			}

			// Now the branch protection check failed, check whether the failure could be skipped (skip by setting err = nil)

			// * when doing Auto Merge (Scheduled Merge After Checks Succeed), skip the branch protection check
			if mergeCheckType == MergeCheckTypeAuto {
				err = nil
			}

			// * if admin tries to "Force Merge", they could sometimes skip the branch protection check
			if adminForceMerge {
				isRepoAdmin, errForceMerge := access_model.IsUserRepoAdmin(ctx, pr.BaseRepo, doer)
				if errForceMerge != nil {
					return fmt.Errorf("IsUserRepoAdmin failed, repo: %v, doer: %v, err: %w", pr.BaseRepoID, doer.ID, errForceMerge)
				}

				protectedBranchRule, errForceMerge := git_model.GetFirstMatchProtectedBranchRule(ctx, pr.BaseRepoID, pr.BaseBranch)
				if errForceMerge != nil {
					return fmt.Errorf("GetFirstMatchProtectedBranchRule failed, repo: %v, base branch: %v, err: %w", pr.BaseRepoID, pr.BaseBranch, errForceMerge)
				}

				// if doer is admin and the "Force Merge" is not blocked, then clear the branch protection check error
				blockAdminForceMerge := protectedBranchRule != nil && protectedBranchRule.BlockAdminMergeOverride
				if isRepoAdmin && !blockAdminForceMerge {
					err = nil
				}
			}

			// If there is still a branch protection check error, return it
			if err != nil {
				return err
			}
		}

		if _, err := isSignedIfRequired(ctx, pr, doer); err != nil {
			return err
		}

		if noDeps, err := issues_model.IssueNoDependenciesLeft(ctx, pr.Issue); err != nil {
			return err
		} else if !noDeps {
			return ErrDependenciesLeft
		}

		return nil
	})
}

// isSignedIfRequired check if merge will be signed if required
func isSignedIfRequired(ctx context.Context, pr *issues_model.PullRequest, doer *user_model.User) (bool, error) {
	pb, err := git_model.GetFirstMatchProtectedBranchRule(ctx, pr.BaseRepoID, pr.BaseBranch)
	if err != nil {
		return false, err
	}

	if pb == nil || !pb.RequireSignedCommits {
		return true, nil
	}

	sign, _, _, err := asymkey_service.SignMerge(ctx, pr, doer, pr.BaseRepo.RepoPath(), pr.BaseBranch, pr.GetGitHeadRefName())

	return sign, err
}

// markPullRequestAsMergeable checks if pull request is possible to leaving checking status,
// and set to be either conflict or mergeable.
func markPullRequestAsMergeable(ctx context.Context, pr *issues_model.PullRequest) {
	// If the status has not been changed to conflict by testPullRequestTmpRepoBranchMergeable then we are mergeable
	if pr.Status == issues_model.PullRequestStatusChecking {
		pr.Status = issues_model.PullRequestStatusMergeable
	}

	// Make sure there is no waiting test to process before leaving the checking status.
	has, err := prPatchCheckerQueue.Has(strconv.FormatInt(pr.ID, 10))
	if err != nil {
		log.Error("Unable to check if the queue is waiting to reprocess %-v. Error: %v", pr, err)
	}

	if has {
		log.Trace("Not updating status for %-v as it is due to be rechecked", pr)
		return
	}

	if err := pr.UpdateColsIfNotMerged(ctx, "merge_base", "status", "conflicted_files", "changed_protected_files"); err != nil {
		log.Error("Update[%-v]: %v", pr, err)
	}

	// if there is a scheduled merge for this pull request, start the auto merge check (again)
	exist, _, err := pull.GetScheduledMergeByPullID(ctx, pr.ID)
	if err != nil {
		log.Error("GetScheduledMergeByPullID[%-v]: %v", pr, err)
		return
	} else if !exist {
		return
	}
	automergequeue.StartPRCheckAndAutoMerge(ctx, pr)
}

// getMergeCommit checks if a pull request has been merged
// Returns the git.Commit of the pull request if merged
func getMergeCommit(ctx context.Context, pr *issues_model.PullRequest) (*git.Commit, error) {
	if err := pr.LoadBaseRepo(ctx); err != nil {
		return nil, fmt.Errorf("unable to load base repo for %s: %w", pr, err)
	}

	prHeadRef := pr.GetGitHeadRefName()

	// Check if the pull request is merged into BaseBranch
	if _, _, err := git.NewCommand("merge-base", "--is-ancestor").
		AddDynamicArguments(prHeadRef, pr.BaseBranch).
		RunStdString(ctx, &git.RunOpts{Dir: pr.BaseRepo.RepoPath()}); err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			// prHeadRef is not an ancestor of the base branch
			return nil, nil
		}
		// Errors are signaled by a non-zero status that is not 1
		return nil, fmt.Errorf("%-v git merge-base --is-ancestor: %w", pr, err)
	}

	// If merge-base successfully exits then prHeadRef is an ancestor of pr.BaseBranch

	// Find the head commit id
	prHeadCommitID, err := git.GetFullCommitID(ctx, pr.BaseRepo.RepoPath(), prHeadRef)
	if err != nil {
		return nil, fmt.Errorf("GetFullCommitID(%s) in %s: %w", prHeadRef, pr.BaseRepo.FullName(), err)
	}

	gitRepo, err := gitrepo.OpenRepository(ctx, pr.BaseRepo)
	if err != nil {
		return nil, fmt.Errorf("%-v OpenRepository: %w", pr.BaseRepo, err)
	}
	defer gitRepo.Close()

	objectFormat := git.ObjectFormatFromName(pr.BaseRepo.ObjectFormatName)

	// Get the commit from BaseBranch where the pull request got merged
	mergeCommit, _, err := git.NewCommand("rev-list", "--ancestry-path", "--merges", "--reverse").
		AddDynamicArguments(prHeadCommitID+".."+pr.BaseBranch).
		RunStdString(ctx, &git.RunOpts{Dir: pr.BaseRepo.RepoPath()})
	if err != nil {
		return nil, fmt.Errorf("git rev-list --ancestry-path --merges --reverse: %w", err)
	} else if len(mergeCommit) < objectFormat.FullLength() {
		// PR was maybe fast-forwarded, so just use last commit of PR
		mergeCommit = prHeadCommitID
	}
	mergeCommit = strings.TrimSpace(mergeCommit)

	commit, err := gitRepo.GetCommit(mergeCommit)
	if err != nil {
		return nil, fmt.Errorf("GetMergeCommit[%s]: %w", mergeCommit, err)
	}

	return commit, nil
}

// manuallyMerged checks if a pull request got manually merged
// When a pull request got manually merged mark the pull request as merged
func manuallyMerged(ctx context.Context, pr *issues_model.PullRequest) bool {
	if err := pr.LoadBaseRepo(ctx); err != nil {
		log.Error("%-v LoadBaseRepo: %v", pr, err)
		return false
	}

	if unit, err := pr.BaseRepo.GetUnit(ctx, unit.TypePullRequests); err == nil {
		config := unit.PullRequestsConfig()
		if !config.AutodetectManualMerge {
			return false
		}
	} else {
		log.Error("%-v BaseRepo.GetUnit(unit.TypePullRequests): %v", pr, err)
		return false
	}

	commit, err := getMergeCommit(ctx, pr)
	if err != nil {
		log.Error("%-v getMergeCommit: %v", pr, err)
		return false
	}

	if commit == nil {
		// no merge commit found
		return false
	}

	merger, _ := user_model.GetUserByEmail(ctx, commit.Author.Email)

	// When the commit author is unknown set the BaseRepo owner as merger
	if merger == nil {
		if pr.BaseRepo.Owner == nil {
			if err = pr.BaseRepo.LoadOwner(ctx); err != nil {
				log.Error("%-v BaseRepo.LoadOwner: %v", pr, err)
				return false
			}
		}
		merger = pr.BaseRepo.Owner
	}

	if merged, err := SetMerged(ctx, pr, commit.ID.String(), timeutil.TimeStamp(commit.Author.When.Unix()), merger, issues_model.PullRequestStatusManuallyMerged); err != nil {
		log.Error("%-v setMerged : %v", pr, err)
		return false
	} else if !merged {
		return false
	}

	notify_service.MergePullRequest(ctx, merger, pr)

	log.Info("manuallyMerged[%-v]: Marked as manually merged into %s/%s by commit id: %s", pr, pr.BaseRepo.Name, pr.BaseBranch, commit.ID.String())
	return true
}

// InitializePullRequests checks and tests untested patches of pull requests.
func InitializePullRequests(ctx context.Context) {
	// If we prefer to delay the checks, then no need to do any check during startup, there should be not much difference
	if setting.Repository.PullRequest.DelayCheckForInactiveDays >= 0 {
		return
	}
	prs, err := issues_model.GetPullRequestIDsByCheckStatus(ctx, issues_model.PullRequestStatusChecking)
	if err != nil {
		log.Error("Find Checking PRs: %v", err)
		return
	}
	for _, prID := range prs {
		select {
		case <-ctx.Done():
			return
		default:
			AddPullRequestToCheckQueue(prID)
		}
	}
}

func checkPullRequestMergeable(id int64) {
	ctx := graceful.GetManager().HammerContext()
	releaser, err := globallock.Lock(ctx, getPullWorkingLockKey(id))
	if err != nil {
		log.Error("lock.Lock(): %v", err)
		return
	}
	defer releaser()

	ctx, _, finished := process.GetManager().AddContext(ctx, fmt.Sprintf("Test PR[%d] from patch checking queue", id))
	defer finished()

	pr, err := issues_model.GetPullRequestByID(ctx, id)
	if err != nil {
		log.Error("Unable to GetPullRequestByID[%d] for checkPullRequestMergeable: %v", id, err)
		return
	}

	log.Trace("Testing %-v", pr)
	defer func() {
		log.Trace("Done testing %-v (status: %s)", pr, pr.Status)
	}()

	if pr.HasMerged {
		log.Trace("%-v is already merged (status: %s, merge commit: %s)", pr, pr.Status, pr.MergedCommitID)
		return
	}

	if manuallyMerged(ctx, pr) {
		log.Trace("%-v is manually merged (status: %s, merge commit: %s)", pr, pr.Status, pr.MergedCommitID)
		return
	}

	if err := testPullRequestBranchMergeable(pr); err != nil {
		log.Error("testPullRequestTmpRepoBranchMergeable[%-v]: %v", pr, err)
		pr.Status = issues_model.PullRequestStatusError
		if err := pr.UpdateCols(ctx, "status"); err != nil {
			log.Error("update pr [%-v] status to PullRequestStatusError failed: %v", pr, err)
		}
		return
	}
	markPullRequestAsMergeable(ctx, pr)
}

// CheckPRsForBaseBranch check all pulls with baseBrannch
func CheckPRsForBaseBranch(ctx context.Context, baseRepo *repo_model.Repository, baseBranchName string) error {
	prs, err := issues_model.GetUnmergedPullRequestsByBaseInfo(ctx, baseRepo.ID, baseBranchName)
	if err != nil {
		return err
	}
	for _, pr := range prs {
		StartPullRequestCheckImmediately(ctx, pr)
	}
	return nil
}

// Init runs the task queue to test all the checking status pull requests
func Init() error {
	prPatchCheckerQueue = queue.CreateUniqueQueue(graceful.GetManager().ShutdownContext(), "pr_patch_checker", func(items ...string) []string {
		for _, s := range items {
			id, _ := strconv.ParseInt(s, 10, 64)
			checkPullRequestMergeable(id)
		}
		return nil
	})

	if prPatchCheckerQueue == nil {
		return errors.New("unable to create pr_patch_checker queue")
	}

	go graceful.GetManager().RunWithCancel(prPatchCheckerQueue)
	go graceful.GetManager().RunWithShutdownContext(InitializePullRequests)
	return nil
}
