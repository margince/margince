// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/platform/recordmerge"

const (
	columnContactID     = "contact_id"
	columnCompanyID     = "company_id"
	contactEmailsField  = "emails"
	contactPhonesField  = "phones"
	companyDomainsField = "domains"
	fieldSource         = "source"
	addressKindDomain   = "domain"
)

const (
	archiveStateKey   = "archived"
	archiveRecordKey  = "record"
	mergeSourceKey    = recordmerge.SourceKey
	mergeTargetKey    = recordmerge.TargetKey
	mergeSubjectKey   = "subject"
	mergeOtherKey     = "other"
	mergeEntityKey    = "entity"
	recordKindParam   = recordmerge.KindKey
	factValueKeyParam = "key"
)
