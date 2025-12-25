// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package admin

import (
	"net/http"

	"code.gitea.io/gitea/models/webhook"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

const (
	// tplAdminHooks template path to render hook settings
	tplAdminHooks templates.TplName = "admin/hooks"
)

// DefaultOrSystemWebhooks renders both admin default and system webhook list pages
func DefaultOrSystemWebhooks(ctx *context.Context) {
	var err error

	ctx.Data["Title"] = ctx.Tr("Webhooks")
	ctx.Data["PageIsAdminSystemHooks"] = true
	ctx.Data["PageIsAdminDefaultHooks"] = true

	def := make(map[string]any, len(ctx.Data))
	sys := make(map[string]any, len(ctx.Data))
	for k, v := range ctx.Data {
		def[k] = v
		sys[k] = v
	}

	sys["Title"] = ctx.Tr("System Webhooks")
	sys["Description"] = ctx.Tr("Webhooks automatically make HTTP POST requests to a server when certain Gitea events trigger. Webhooks defined here will act on all repositories on the system, so please consider any performance implications this may have. Read more in the <a target=\"_blank\" rel=\"noopener\" href=\"%s\">webhooks guide</a>.", "https://docs.gitea.com/usage/webhooks")
	sys["Webhooks"], err = webhook.GetSystemWebhooks(ctx, optional.None[bool]())
	sys["BaseLink"] = setting.AppSubURL + "/-/admin/hooks"
	sys["BaseLinkNew"] = setting.AppSubURL + "/-/admin/system-hooks"
	if err != nil {
		ctx.ServerError("GetWebhooksAdmin", err)
		return
	}

	def["Title"] = ctx.Tr("Default Webhooks")
	def["Description"] = ctx.Tr("Webhooks automatically make HTTP POST requests to a server when certain Gitea events trigger. Webhooks defined here are defaults and will be copied into all new repositories. Read more in the <a target=\"_blank\" rel=\"noopener\" href=\"%s\">webhooks guide</a>.", "https://docs.gitea.com/usage/webhooks")
	def["Webhooks"], err = webhook.GetDefaultWebhooks(ctx)
	def["BaseLink"] = setting.AppSubURL + "/-/admin/hooks"
	def["BaseLinkNew"] = setting.AppSubURL + "/-/admin/default-hooks"
	if err != nil {
		ctx.ServerError("GetWebhooksAdmin", err)
		return
	}

	ctx.Data["DefaultWebhooks"] = def
	ctx.Data["SystemWebhooks"] = sys

	ctx.HTML(http.StatusOK, tplAdminHooks)
}

// DeleteDefaultOrSystemWebhook handler to delete an admin-defined system or default webhook
func DeleteDefaultOrSystemWebhook(ctx *context.Context) {
	if err := webhook.DeleteDefaultSystemWebhook(ctx, ctx.FormInt64("id")); err != nil {
		ctx.Flash.Error("DeleteDefaultWebhook: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("The webhook has been removed."))
	}

	ctx.JSONRedirect(setting.AppSubURL + "/-/admin/hooks")
}
