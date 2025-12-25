// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	git_model "code.gitea.io/gitea/models/git"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/optional"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/routers/utils"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/forms"
	pull_service "code.gitea.io/gitea/services/pull"
	release_service "code.gitea.io/gitea/services/release"
	repo_service "code.gitea.io/gitea/services/repository"
)

const (
	tplBranch templates.TplName = "repo/branch/list"
)

// Branches render repository branch page
func Branches(ctx *context.Context) {
	ctx.Data["Title"] = "Branches"
	ctx.Data["AllowsPulls"] = ctx.Repo.Repository.AllowsPulls(ctx)
	ctx.Data["IsWriter"] = ctx.Repo.CanWrite(unit.TypeCode)
	ctx.Data["IsMirror"] = ctx.Repo.Repository.IsMirror
	ctx.Data["CanPull"] = ctx.Repo.CanWrite(unit.TypeCode) ||
		(ctx.IsSigned && repo_model.HasForkedRepo(ctx, ctx.Doer.ID, ctx.Repo.Repository.ID))
	ctx.Data["PageIsViewCode"] = true
	ctx.Data["PageIsBranches"] = true

	page := max(ctx.FormInt("page"), 1)
	pageSize := setting.Git.BranchesRangeSize

	kw := ctx.FormString("q")

	defaultBranch, branches, branchesCount, err := repo_service.LoadBranches(ctx, ctx.Repo.Repository, ctx.Repo.GitRepo, optional.None[bool](), kw, page, pageSize)
	if err != nil {
		ctx.ServerError("LoadBranches", err)
		return
	}

	commitIDs := []string{defaultBranch.DBBranch.CommitID}
	for _, branch := range branches {
		commitIDs = append(commitIDs, branch.DBBranch.CommitID)
	}

	commitStatuses, err := git_model.GetLatestCommitStatusForRepoCommitIDs(ctx, ctx.Repo.Repository.ID, commitIDs)
	if err != nil {
		ctx.ServerError("LoadBranches", err)
		return
	}
	if !ctx.Repo.CanRead(unit.TypeActions) {
		for key := range commitStatuses {
			git_model.CommitStatusesHideActionsURL(ctx, commitStatuses[key])
		}
	}

	commitStatus := make(map[string]*git_model.CommitStatus)
	for commitID, cs := range commitStatuses {
		commitStatus[commitID] = git_model.CalcCommitStatus(cs)
	}

	ctx.Data["Keyword"] = kw
	ctx.Data["Branches"] = branches
	ctx.Data["CommitStatus"] = commitStatus
	ctx.Data["CommitStatuses"] = commitStatuses
	ctx.Data["DefaultBranchBranch"] = defaultBranch
	pager := context.NewPagination(int(branchesCount), pageSize, page, 5)
	pager.AddParamFromRequest(ctx.Req)
	ctx.Data["Page"] = pager
	ctx.HTML(http.StatusOK, tplBranch)
}

// DeleteBranchPost responses for delete merged branch
func DeleteBranchPost(ctx *context.Context) {
	defer jsonRedirectBranches(ctx)
	branchName := ctx.FormString("name")

	if err := repo_service.DeleteBranch(ctx, ctx.Doer, ctx.Repo.Repository, ctx.Repo.GitRepo, branchName, nil); err != nil {
		switch {
		case git.IsErrBranchNotExist(err):
			log.Debug("DeleteBranch: Can't delete non existing branch '%s'", branchName)
			ctx.Flash.Error(ctx.Tr("Failed to delete branch \"%s\".", branchName))
		case errors.Is(err, repo_service.ErrBranchIsDefault):
			log.Debug("DeleteBranch: Can't delete default branch '%s'", branchName)
			ctx.Flash.Error(ctx.Tr("Branch \"%s\" is the default branch. It cannot be deleted.", branchName))
		case errors.Is(err, git_model.ErrBranchIsProtected):
			log.Debug("DeleteBranch: Can't delete protected branch '%s'", branchName)
			ctx.Flash.Error(ctx.Tr("Branch \"%s\" is protected. It cannot be deleted.", branchName))
		default:
			log.Error("DeleteBranch: %v", err)
			ctx.Flash.Error(ctx.Tr("Failed to delete branch \"%s\".", branchName))
		}

		return
	}

	ctx.Flash.Success(ctx.Tr("Branch \"%s\" has been deleted.", branchName))
}

// RestoreBranchPost responses for delete merged branch
func RestoreBranchPost(ctx *context.Context) {
	defer jsonRedirectBranches(ctx)

	branchID := ctx.FormInt64("branch_id")
	branchName := ctx.FormString("name")

	deletedBranch, err := git_model.GetDeletedBranchByID(ctx, ctx.Repo.Repository.ID, branchID)
	if err != nil {
		log.Error("GetDeletedBranchByID: %v", err)
		ctx.Flash.Error(ctx.Tr("Failed to restore branch \"%s\".", branchName))
		return
	} else if deletedBranch == nil {
		log.Debug("RestoreBranch: Can't restore branch[%d] '%s', as it does not exist", branchID, branchName)
		ctx.Flash.Error(ctx.Tr("Failed to restore branch \"%s\".", branchName))
		return
	}

	if err := gitrepo.Push(ctx, ctx.Repo.Repository, ctx.Repo.Repository, git.PushOptions{
		Branch: fmt.Sprintf("%s:%s%s", deletedBranch.CommitID, git.BranchPrefix, deletedBranch.Name),
		Env:    repo_module.PushingEnvironment(ctx.Doer, ctx.Repo.Repository),
	}); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			log.Debug("RestoreBranch: Can't restore branch '%s', since one with same name already exist", deletedBranch.Name)
			ctx.Flash.Error(ctx.Tr("A branch named \"%s\" already exists.", deletedBranch.Name))
			return
		}
		log.Error("RestoreBranch: CreateBranch: %v", err)
		ctx.Flash.Error(ctx.Tr("Failed to restore branch \"%s\".", deletedBranch.Name))
		return
	}

	objectFormat := git.ObjectFormatFromName(ctx.Repo.Repository.ObjectFormatName)

	// Don't return error below this
	if err := repo_service.PushUpdate(
		&repo_module.PushUpdateOptions{
			RefFullName:  git.RefNameFromBranch(deletedBranch.Name),
			OldCommitID:  objectFormat.EmptyObjectID().String(),
			NewCommitID:  deletedBranch.CommitID,
			PusherID:     ctx.Doer.ID,
			PusherName:   ctx.Doer.Name,
			RepoUserName: ctx.Repo.Owner.Name,
			RepoName:     ctx.Repo.Repository.Name,
		}); err != nil {
		log.Error("RestoreBranch: Update: %v", err)
	}

	ctx.Flash.Success(ctx.Tr("Branch \"%s\" has been restored.", deletedBranch.Name))
}

func jsonRedirectBranches(ctx *context.Context) {
	ctx.JSONRedirect(ctx.Repo.RepoLink + "/branches?page=" + url.QueryEscape(ctx.FormString("page")))
}

// CreateBranch creates new branch in repository
func CreateBranch(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.NewBranchForm)
	if !ctx.Repo.CanCreateBranch() {
		ctx.NotFound(nil)
		return
	}

	if ctx.HasError() {
		ctx.Flash.Error(ctx.GetErrMsg())
		ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
		return
	}

	var err error

	if form.CreateTag {
		target := ctx.Repo.CommitID
		if ctx.Repo.RefFullName.IsBranch() {
			target = ctx.Repo.BranchName
		}
		err = release_service.CreateNewTag(ctx, ctx.Doer, ctx.Repo.Repository, target, form.NewBranchName, "")
	} else if ctx.Repo.RefFullName.IsBranch() {
		err = repo_service.CreateNewBranch(ctx, ctx.Doer, ctx.Repo.Repository, ctx.Repo.BranchName, form.NewBranchName)
	} else {
		err = repo_service.CreateNewBranchFromCommit(ctx, ctx.Doer, ctx.Repo.Repository, ctx.Repo.CommitID, form.NewBranchName)
	}
	if err != nil {
		if release_service.IsErrProtectedTagName(err) {
			ctx.Flash.Error(ctx.Tr("The tag name is protected."))
			ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
			return
		}

		if release_service.IsErrTagAlreadyExists(err) {
			e := err.(release_service.ErrTagAlreadyExists)
			ctx.Flash.Error(ctx.Tr("Branch \"%s\" cannot be created as a tag with same name already exists in the repository.", e.TagName))
			ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
			return
		}
		if git_model.IsErrBranchAlreadyExists(err) || git.IsErrPushOutOfDate(err) {
			ctx.Flash.Error(ctx.Tr("Branch \"%s\" already exists in this repository.", form.NewBranchName))
			ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
			return
		}
		if git_model.IsErrBranchNameConflict(err) {
			e := err.(git_model.ErrBranchNameConflict)
			ctx.Flash.Error(ctx.Tr("Branch name \"%s\" conflicts with the already existing branch \"%s\".", form.NewBranchName, e.BranchName))
			ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
			return
		}
		if git.IsErrPushRejected(err) {
			e := err.(*git.ErrPushRejected)
			if len(e.Message) == 0 {
				ctx.Flash.Error(ctx.Tr("The change was rejected by the server without a message. Please check Git Hooks."))
			} else {
				flashError, err := ctx.RenderToHTML(tplAlertDetails, map[string]any{
					"Message": ctx.Tr("The change was rejected by the server. Please check Git Hooks."),
					"Summary": ctx.Tr("Full Rejection Message:"),
					"Details": utils.SanitizeFlashErrorString(e.Message),
				})
				if err != nil {
					ctx.ServerError("UpdatePullRequest.HTMLString", err)
					return
				}
				ctx.Flash.Error(flashError)
			}
			ctx.Redirect(ctx.Repo.RepoLink + "/src/" + ctx.Repo.RefTypeNameSubURL())
			return
		}

		ctx.ServerError("CreateNewBranch", err)
		return
	}

	if form.CreateTag {
		ctx.Flash.Success(ctx.Tr("Tag \"%s\" has been created.", form.NewBranchName))
		ctx.Redirect(ctx.Repo.RepoLink + "/src/tag/" + util.PathEscapeSegments(form.NewBranchName))
		return
	}

	ctx.Flash.Success(ctx.Tr("Branch \"%s\" has been created.", form.NewBranchName))
	ctx.Redirect(ctx.Repo.RepoLink + "/src/branch/" + util.PathEscapeSegments(form.NewBranchName) + "/" + util.PathEscapeSegments(form.CurrentPath))
}

func MergeUpstream(ctx *context.Context) {
	branchName := ctx.FormString("branch")
	_, err := repo_service.MergeUpstream(ctx, ctx.Doer, ctx.Repo.Repository, branchName, false)
	if err != nil {
		if errors.Is(err, util.ErrNotExist) {
			ctx.JSONErrorNotFound()
			return
		} else if pull_service.IsErrMergeConflicts(err) {
			ctx.JSONError(ctx.Tr("Merge Failed: There was a conflict while merging. Hint: Try a different strategy."))
			return
		}
		ctx.ServerError("MergeUpstream", err)
		return
	}
	ctx.JSONRedirect("")
}
