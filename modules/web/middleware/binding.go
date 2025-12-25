// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package middleware

import (
	"reflect"
	"strings"

	"code.gitea.io/gitea/modules/translation"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/validation"

	"gitea.com/go-chi/binding"
)

// Form form binding interface
type Form interface {
	binding.Validator
}

func init() {
	binding.SetNameMapper(util.ToSnakeCase)
}

// AssignForm assign form values back to the template data.
func AssignForm(form any, data map[string]any) {
	typ := reflect.TypeOf(form)
	val := reflect.ValueOf(form)

	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		fieldName := field.Tag.Get("form")
		// Allow ignored fields in the struct
		if fieldName == "-" {
			continue
		} else if len(fieldName) == 0 {
			fieldName = util.ToSnakeCase(field.Name)
		}

		data[fieldName] = val.Field(i).Interface()
	}
}

func getRuleBody(field reflect.StructField, prefix string) string {
	for rule := range strings.SplitSeq(field.Tag.Get("binding"), ";") {
		if strings.HasPrefix(rule, prefix) {
			return rule[len(prefix) : len(rule)-1]
		}
	}
	return ""
}

// GetSize get size int form tag
func GetSize(field reflect.StructField) string {
	return getRuleBody(field, "Size(")
}

// GetMinSize get minimal size in form tag
func GetMinSize(field reflect.StructField) string {
	return getRuleBody(field, "MinSize(")
}

// GetMaxSize get max size in form tag
func GetMaxSize(field reflect.StructField) string {
	return getRuleBody(field, "MaxSize(")
}

// GetInclude get include in form tag
func GetInclude(field reflect.StructField) string {
	return getRuleBody(field, "Include(")
}

// Validate validate
func Validate(errs binding.Errors, data map[string]any, f Form, l translation.Locale) binding.Errors {
	if errs.Len() == 0 {
		return errs
	}

	data["HasError"] = true
	// If the field with name errs[0].FieldNames[0] is not found in form
	// somehow, some code later on will panic on Data["ErrorMsg"].(string).
	// So initialize it to some default.
	data["ErrorMsg"] = l.Tr("Unknown error:")
	AssignForm(f, data)

	typ := reflect.TypeOf(f)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if field, ok := typ.FieldByName(errs[0].FieldNames[0]); ok {
		fieldName := field.Tag.Get("form")
		if fieldName != "-" {
			data["Err_"+field.Name] = true

			trName := field.Tag.Get("locale")
			if len(trName) == 0 {
				trName = l.TrString("form." + field.Name)
			} else {
				trName = l.TrString(trName)
			}

			switch errs[0].Classification {
			case binding.ERR_REQUIRED:
				data["ErrorMsg"] = trName + l.TrString(" cannot be empty.")
			case binding.ERR_ALPHA_DASH:
				data["ErrorMsg"] = trName + l.TrString(" should contain only alphanumeric, dash ('-') and underscore ('_') characters.")
			case binding.ERR_ALPHA_DASH_DOT:
				data["ErrorMsg"] = trName + l.TrString(" should contain only alphanumeric, dash ('-'), underscore ('_') and dot ('.') characters.")
			case validation.ErrGitRefName:
				data["ErrorMsg"] = trName + l.TrString(" must be a well-formed Git reference name.")
			case binding.ERR_SIZE:
				data["ErrorMsg"] = trName + l.TrString(" must be size %s.", GetSize(field))
			case binding.ERR_MIN_SIZE:
				data["ErrorMsg"] = trName + l.TrString(" must contain at least %s characters.", GetMinSize(field))
			case binding.ERR_MAX_SIZE:
				data["ErrorMsg"] = trName + l.TrString(" must contain at most %s characters.", GetMaxSize(field))
			case binding.ERR_EMAIL:
				data["ErrorMsg"] = trName + l.TrString(" is not a valid email address.")
			case binding.ERR_URL:
				data["ErrorMsg"] = trName + l.TrString("\"%s\" is not a valid URL.", errs[0].Message)
			case binding.ERR_INCLUDE:
				data["ErrorMsg"] = trName + l.TrString(" must contain substring \"%s\".", GetInclude(field))
			case validation.ErrGlobPattern:
				data["ErrorMsg"] = trName + l.TrString(" glob pattern is invalid: %s.", errs[0].Message)
			case validation.ErrRegexPattern:
				data["ErrorMsg"] = trName + l.TrString(" regex pattern is invalid: %s.", errs[0].Message)
			case validation.ErrUsername:
				data["ErrorMsg"] = trName + l.TrString(" can only contain alphanumeric characters ('0-9','a-z','A-Z'), dash ('-'), underscore ('_') and dot ('.'). It cannot begin or end with non-alphanumeric characters, and consecutive non-alphanumeric characters are also forbidden.")
			case validation.ErrInvalidGroupTeamMap:
				data["ErrorMsg"] = trName + l.TrString(" mapping is invalid: %s", errs[0].Message)
			default:
				msg := errs[0].Classification
				if msg != "" && errs[0].Message != "" {
					msg += ": "
				}

				msg += errs[0].Message
				if msg == "" {
					msg = l.TrString("Unknown error:")
				}
				data["ErrorMsg"] = trName + ": " + msg
			}
			return errs
		}
	}
	return errs
}
