// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// clearablePersonColumns maps the wire fields a person restore may set to NULL
// onto the column holding each, with the row's current value for the audit
// image. The column names are literals here and never come from a caller, so
// nothing caller-supplied reaches the UPDATE text.
//
// A field absent from this map cannot be cleared, and the reversal path refuses
// rather than reporting a success it did not have.
//
//nolint:goconst // the map keys are wire field names and the values are COLUMN names; each is its own vocabulary, and the constants goconst names are filter params and report fields that spell the same words by coincidence
func clearablePersonColumns(current crmcontracts.Person) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"first_name": {"first_name", current.FirstName},
		"last_name":  {"last_name", current.LastName},
		"title":      {"title", current.Title},
		"owner_id":   {ownerIDColumn, current.OwnerId},
	}
}

// clearableOrganizationColumns names the wire fields an organization restore may
// set to NULL, with literal column names — nothing caller-supplied reaches the
// UPDATE text. A field absent here cannot be cleared, and the reversal path
// refuses rather than reporting a success it did not have.
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns
func clearableOrganizationColumns(current crmcontracts.Organization) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"legal_name":    {"legal_name", current.LegalName},
		"description":   {"description", current.Description},
		"industry":      {"industry", current.Industry},
		"size_band":     {"size_band", current.SizeBand},
		"linkedin_url":  {"linkedin_url", current.LinkedinUrl},
		"owner_id":      {ownerIDColumn, current.OwnerId},
		"parent_org_id": {"parent_org_id", current.ParentOrgId},
	}
}

// clearableLeadColumns names the wire fields a lead restore may set to NULL,
// with literal column names. `status` and the score pair are absent on purpose:
// a lead's status is a lifecycle position rather than a value, and the score
// override is sticky by design (clearing its reason resumes recompute, which is
// a decision rather than a field edit).
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns
func clearableLeadColumns(current crmcontracts.Lead) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"title":             {"title", current.Title},
		"company_name":      {leadCompanyColumn, current.CompanyName},
		"candidate_org_key": {"candidate_org_key", current.CandidateOrgKey},
		"project_id":        {"project_id", current.ProjectId},
		"owner_id":          {ownerIDColumn, current.OwnerId},
	}
}
