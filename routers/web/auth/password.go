// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"net/http"

	"code.gitea.io/gitea/models/auth"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/auth/password"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/modules/web/middleware"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/forms"
	"code.gitea.io/gitea/services/mailer"
	user_service "code.gitea.io/gitea/services/user"
)

var (
	// tplMustChangePassword template for updating a user's password
	tplMustChangePassword templates.TplName = "user/auth/change_passwd"
	tplForgotPassword     templates.TplName = "user/auth/forgot_passwd"
	tplResetPassword      templates.TplName = "user/auth/reset_passwd"
)

// ForgotPasswd render the forget password page
func ForgotPasswd(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("Forgot Password")

	if setting.MailService == nil {
		log.Warn("no mail service configured")
		ctx.Data["IsResetDisable"] = true
		ctx.HTML(http.StatusOK, tplForgotPassword)
		return
	}

	ctx.Data["Email"] = ctx.FormString("email")

	ctx.Data["IsResetRequest"] = true
	ctx.HTML(http.StatusOK, tplForgotPassword)
}

// ForgotPasswdPost response for forget password request
func ForgotPasswdPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("Forgot Password")

	if setting.MailService == nil {
		ctx.NotFound(nil)
		return
	}
	ctx.Data["IsResetRequest"] = true

	email := ctx.FormString("email")
	ctx.Data["Email"] = email

	u, err := user_model.GetUserByEmail(ctx, email)
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			ctx.Data["ResetPwdCodeLives"] = timeutil.MinutesToFriendly(setting.Service.ResetPwdCodeLives, ctx.Locale)
			ctx.Data["IsResetSent"] = true
			ctx.HTML(http.StatusOK, tplForgotPassword)
			return
		}

		ctx.ServerError("user.ResetPasswd(check existence)", err)
		return
	}

	if !u.IsLocal() && !u.IsOAuth2() {
		ctx.Data["Err_Email"] = true
		ctx.RenderWithErr(ctx.Tr("Non-local users cannot update their password through the Gitea web interface."), tplForgotPassword, nil)
		return
	}

	if ctx.Cache.IsExist("MailResendLimit_" + u.LowerName) {
		ctx.Data["ResendLimited"] = true
		ctx.HTML(http.StatusOK, tplForgotPassword)
		return
	}

	mailer.SendResetPasswordMail(u)

	if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
		log.Error("Set cache(MailResendLimit) fail: %v", err)
	}

	ctx.Data["ResetPwdCodeLives"] = timeutil.MinutesToFriendly(setting.Service.ResetPwdCodeLives, ctx.Locale)
	ctx.Data["IsResetSent"] = true
	ctx.HTML(http.StatusOK, tplForgotPassword)
}

func commonResetPassword(ctx *context.Context) (*user_model.User, *auth.TwoFactor) {
	code := ctx.FormString("code")

	ctx.Data["Title"] = ctx.Tr("Account Recovery")
	ctx.Data["Code"] = code

	if nil != ctx.Doer {
		ctx.Data["user_signed_in"] = true
	}

	if len(code) == 0 {
		ctx.Flash.Error(ctx.Tr("Your confirmation code is invalid or has expired. Click <a href=\"%s\">here</a> to start a new session.", setting.AppSubURL+"/user/forgot_password"), true)
		return nil, nil
	}

	// Fail early, don't frustrate the user
	u := user_model.VerifyUserTimeLimitCode(ctx, &user_model.TimeLimitCodeOptions{Purpose: user_model.TimeLimitCodeResetPassword}, code)
	if u == nil {
		ctx.Flash.Error(ctx.Tr("Your confirmation code is invalid or has expired. Click <a href=\"%s\">here</a> to start a new session.", setting.AppSubURL+"/user/forgot_password"), true)
		return nil, nil
	}

	twofa, err := auth.GetTwoFactorByUID(ctx, u.ID)
	if err != nil {
		if !auth.IsErrTwoFactorNotEnrolled(err) {
			ctx.HTTPError(http.StatusInternalServerError, "CommonResetPassword", err.Error())
			return nil, nil
		}
	} else {
		ctx.Data["has_two_factor"] = true
		ctx.Data["scratch_code"] = ctx.FormBool("scratch_code")
	}

	// Show the user that they are affecting the account that they intended to
	ctx.Data["user_email"] = u.Email

	if nil != ctx.Doer && u.ID != ctx.Doer.ID {
		ctx.Flash.Error(ctx.Tr("You are signed in as %s, but the account recovery link is meant for %s", ctx.Doer.Email, u.Email), true)
		return nil, nil
	}

	return u, twofa
}

// ResetPasswd render the account recovery page
func ResetPasswd(ctx *context.Context) {
	ctx.Data["IsResetForm"] = true

	commonResetPassword(ctx)
	if ctx.Written() {
		return
	}

	ctx.HTML(http.StatusOK, tplResetPassword)
}

// ResetPasswdPost response from account recovery request
func ResetPasswdPost(ctx *context.Context) {
	u, twofa := commonResetPassword(ctx)
	if ctx.Written() {
		return
	}

	if u == nil {
		// Flash error has been set
		ctx.HTML(http.StatusOK, tplResetPassword)
		return
	}

	// Handle two-factor
	regenerateScratchToken := false
	if twofa != nil {
		if ctx.FormBool("scratch_code") {
			if !twofa.VerifyScratchToken(ctx.FormString("token")) {
				ctx.Data["IsResetForm"] = true
				ctx.Data["Err_Token"] = true
				ctx.RenderWithErr(ctx.Tr("Your scratch code is incorrect."), tplResetPassword, nil)
				return
			}
			regenerateScratchToken = true
		} else {
			passcode := ctx.FormString("passcode")
			ok, err := twofa.ValidateTOTP(passcode)
			if err != nil {
				ctx.HTTPError(http.StatusInternalServerError, "ValidateTOTP", err.Error())
				return
			}
			if !ok || twofa.LastUsedPasscode == passcode {
				ctx.Data["IsResetForm"] = true
				ctx.Data["Err_Passcode"] = true
				ctx.RenderWithErr(ctx.Tr("Your passcode is incorrect. If you misplaced your device, use your scratch code to sign in."), tplResetPassword, nil)
				return
			}

			twofa.LastUsedPasscode = passcode
			if err = auth.UpdateTwoFactor(ctx, twofa); err != nil {
				ctx.ServerError("ResetPasswdPost: UpdateTwoFactor", err)
				return
			}
		}
	}

	opts := &user_service.UpdateAuthOptions{
		Password:           optional.Some(ctx.FormString("password")),
		MustChangePassword: optional.Some(false),
	}
	if err := user_service.UpdateAuth(ctx, u, opts); err != nil {
		ctx.Data["IsResetForm"] = true
		ctx.Data["Err_Password"] = true
		switch {
		case errors.Is(err, password.ErrMinLength):
			ctx.RenderWithErr(ctx.Tr("Password length cannot be less than %d characters.", setting.MinPasswordLength), tplResetPassword, nil)
		case errors.Is(err, password.ErrComplexity):
			ctx.RenderWithErr(password.BuildComplexityError(ctx.Locale), tplResetPassword, nil)
		case errors.Is(err, password.ErrIsPwned):
			ctx.RenderWithErr(ctx.Tr("The password you chose is on a <a target=\"_blank\" rel=\"noopener noreferrer\" href=\"%s\">list of stolen passwords</a> previously exposed in public data breaches. Please try again with a different password and consider changing this password elsewhere too.", "https://haveibeenpwned.com/Passwords"), tplResetPassword, nil)
		case password.IsErrIsPwnedRequest(err):
			ctx.RenderWithErr(ctx.Tr("Could not complete request to HaveIBeenPwned"), tplResetPassword, nil)
		default:
			ctx.ServerError("UpdateAuth", err)
		}
		return
	}

	log.Trace("User password reset: %s", u.Name)
	ctx.Data["IsResetFailed"] = true
	remember := len(ctx.FormString("remember")) != 0

	if regenerateScratchToken {
		// Invalidate the scratch token.
		_, err := twofa.GenerateScratchToken()
		if err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}
		if err = auth.UpdateTwoFactor(ctx, twofa); err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}

		handleSignInFull(ctx, u, remember, false)
		if ctx.Written() {
			return
		}
		ctx.Flash.Info(ctx.Tr("You have used your scratch code. You have been redirected to the two-factor settings page so you may remove your device enrollment or generate a new scratch code."))
		ctx.Redirect(setting.AppSubURL + "/user/settings/security")
		return
	}

	handleSignIn(ctx, u, remember)
}

// MustChangePassword renders the page to change a user's password
func MustChangePassword(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("Update your password")
	ctx.Data["ChangePasscodeLink"] = setting.AppSubURL + "/user/settings/change_password"
	ctx.Data["MustChangePassword"] = true
	ctx.HTML(http.StatusOK, tplMustChangePassword)
}

// MustChangePasswordPost response for updating a user's password after their
// account was created by an admin
func MustChangePasswordPost(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.MustChangePasswordForm)
	ctx.Data["Title"] = ctx.Tr("Update your password")
	ctx.Data["ChangePasscodeLink"] = setting.AppSubURL + "/user/settings/change_password"
	if ctx.HasError() {
		ctx.HTML(http.StatusOK, tplMustChangePassword)
		return
	}

	// Make sure only requests for users who are eligible to change their password via
	// this method passes through
	if !ctx.Doer.MustChangePassword {
		ctx.ServerError("MustUpdatePassword", errors.New("cannot update password. Please visit the settings page"))
		return
	}

	if form.Password != form.Retype {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("The passwords do not match."), tplMustChangePassword, &form)
		return
	}

	opts := &user_service.UpdateAuthOptions{
		Password:           optional.Some(form.Password),
		MustChangePassword: optional.Some(false),
	}
	if err := user_service.UpdateAuth(ctx, ctx.Doer, opts); err != nil {
		switch {
		case errors.Is(err, password.ErrMinLength):
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(ctx.Tr("Password length cannot be less than %d characters.", setting.MinPasswordLength), tplMustChangePassword, &form)
		case errors.Is(err, password.ErrComplexity):
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(password.BuildComplexityError(ctx.Locale), tplMustChangePassword, &form)
		case errors.Is(err, password.ErrIsPwned):
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(ctx.Tr("The password you chose is on a <a target=\"_blank\" rel=\"noopener noreferrer\" href=\"%s\">list of stolen passwords</a> previously exposed in public data breaches. Please try again with a different password and consider changing this password elsewhere too.", "https://haveibeenpwned.com/Passwords"), tplMustChangePassword, &form)
		case password.IsErrIsPwnedRequest(err):
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(ctx.Tr("Could not complete request to HaveIBeenPwned"), tplMustChangePassword, &form)
		default:
			ctx.ServerError("UpdateAuth", err)
		}
		return
	}

	ctx.Flash.Success(ctx.Tr("Your password has been updated. Sign in using your new password from now on."))

	log.Trace("User updated password: %s", ctx.Doer.Name)

	if redirectTo := ctx.GetSiteCookie("redirect_to"); redirectTo != "" {
		middleware.DeleteRedirectToCookie(ctx.Resp)
		ctx.RedirectToCurrentSite(redirectTo)
		return
	}

	ctx.Redirect(setting.AppSubURL + "/")
}
