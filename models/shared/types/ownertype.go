// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package types

import "code.gitea.io/gitea/modules/translation"

type OwnerType string

const (
	OwnerTypeSystemGlobal = "system-global"
	OwnerTypeIndividual   = "individual"
	OwnerTypeRepository   = "repository"
	OwnerTypeOrganization = "organization"
)

func (o OwnerType) LocaleString(locale translation.Locale) string {
	switch o {
	case OwnerTypeSystemGlobal:
		return locale.TrString("Global")
	case OwnerTypeIndividual:
		return locale.TrString("Individual")
	case OwnerTypeRepository:
		return locale.TrString("Repository")
	case OwnerTypeOrganization:
		return locale.TrString("Organization")
	}
	return locale.TrString("Unknown")
}
