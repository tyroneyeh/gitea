// Copyright 2025 Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"

	git_model "code.gitea.io/gitea/models/git"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/routers/utils"
	context_service "code.gitea.io/gitea/services/context"
	files_service "code.gitea.io/gitea/services/repository/files"
)

func errorAs[T error](v error) (e T, ok bool) {
	if errors.As(v, &e) {
		return e, true
	}
	return e, false
}

func editorHandleFileOperationErrorRender(ctx *context_service.Context, message, summary, details string) {
	flashError, err := ctx.RenderToHTML(tplAlertDetails, map[string]any{
		"Message": message,
		"Summary": summary,
		"Details": utils.SanitizeFlashErrorString(details),
	})
	if err == nil {
		ctx.JSONError(flashError)
	} else {
		log.Error("RenderToHTML: %v", err)
		ctx.JSONError(message + "\n" + summary + "\n" + utils.SanitizeFlashErrorString(details))
	}
}

func editorHandleFileOperationError(ctx *context_service.Context, targetBranchName string, err error) {
	if errAs := util.ErrorAsTranslatable(err); errAs != nil {
		ctx.JSONError(errAs.Translate(ctx.Locale))
	} else if errAs, ok := errorAs[git.ErrNotExist](err); ok {
		ctx.JSONError(ctx.Tr("The file being modified, \"%s\", no longer exists in this repository.", errAs.RelPath))
	} else if errAs, ok := errorAs[git_model.ErrLFSFileLocked](err); ok {
		ctx.JSONError(ctx.Tr("File \"%s\" is locked by %s.", errAs.Path, errAs.UserName))
	} else if errAs, ok := errorAs[files_service.ErrFilenameInvalid](err); ok {
		ctx.JSONError(ctx.Tr("The filename is invalid: \"%s\".", errAs.Path))
	} else if errAs, ok := errorAs[files_service.ErrFilePathInvalid](err); ok {
		switch errAs.Type {
		case git.EntryModeSymlink:
			ctx.JSONError(ctx.Tr("\"%s\" is a symbolic link. Symbolic links cannot be edited in the web editor.", errAs.Path))
		case git.EntryModeTree:
			ctx.JSONError(ctx.Tr("Filename \"%s\" is already used as a directory name in this repository.", errAs.Path))
		case git.EntryModeBlob:
			ctx.JSONError(ctx.Tr("Directory name \"%s\" is already used as a filename in this repository.", errAs.Path))
		default:
			ctx.JSONError(ctx.Tr("The filename is invalid: \"%s\".", errAs.Path))
		}
	} else if errAs, ok := errorAs[files_service.ErrRepoFileAlreadyExists](err); ok {
		ctx.JSONError(ctx.Tr("A file named \"%s\" already exists in this repository.", errAs.Path))
	} else if errAs, ok := errorAs[git.ErrBranchNotExist](err); ok {
		ctx.JSONError(ctx.Tr("Branch \"%s\" does not exist in this repository.", errAs.Name))
	} else if errAs, ok := errorAs[git_model.ErrBranchAlreadyExists](err); ok {
		ctx.JSONError(ctx.Tr("Branch \"%s\" already exists in this repository.", errAs.BranchName))
	} else if files_service.IsErrCommitIDDoesNotMatch(err) {
		ctx.JSONError(ctx.Tr("The Commit ID does not match the ID when you began editing. Commit into a patch branch and then merge."))
	} else if files_service.IsErrCommitIDDoesNotMatch(err) || git.IsErrPushOutOfDate(err) {
		ctx.JSONError(ctx.Tr("The file contents have changed since you started editing. <a target=\"_blank\" rel=\"noopener noreferrer\" href=\"%s\">Click here</a> to see them or <strong>Commit Changes again</strong> to overwrite them.", ctx.Repo.RepoLink+"/compare/"+util.PathEscapeSegments(ctx.Repo.CommitID)+"..."+util.PathEscapeSegments(targetBranchName)))
	} else if errAs, ok := errorAs[*git.ErrPushRejected](err); ok {
		if errAs.Message == "" {
			ctx.JSONError(ctx.Tr("The change was rejected by the server without a message. Please check Git Hooks."))
		} else {
			editorHandleFileOperationErrorRender(ctx, ctx.Locale.TrString("The change was rejected by the server. Please check Git Hooks."), ctx.Locale.TrString("Full Rejection Message:"), errAs.Message)
		}
	} else if errors.Is(err, util.ErrNotExist) {
		ctx.JSONError(ctx.Tr("The target couldn't be found."))
	} else {
		setting.PanicInDevOrTesting("unclear err %T: %v", err, err)
		editorHandleFileOperationErrorRender(ctx, ctx.Locale.TrString("Failed to commit changes."), ctx.Locale.TrString("Error Message:"), err.Error())
	}
}
