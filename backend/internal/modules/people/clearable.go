// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
)

// clearable is one column a caller may set to NULL, and what the row holds
// there now. The current value is carried so the audit image says what the
// field was cleared FROM.
//
//craft:ignore naked-any the value is whichever type the column holds; the patch seam takes it as the audit image does
type clearable struct {
	column  string
	current any
}

// NotClearableError refuses an explicit null on a field this record cannot set
// to nothing. It maps to 422 through the FieldFault seam.
//
// Refusing matters: the caller sent a null on a field the contract declares
// nullable, so ignoring it would answer 200 having changed nothing — a success
// they cannot trust.
type NotClearableError struct{ Field string }

func (e *NotClearableError) Error() string {
	return e.Field + " cannot be set to null on this record; omit the field to leave it unchanged"
}

// FieldFault names the field the caller tried to clear.
func (e *NotClearableError) FieldFault() (field, code, message string) {
	return e.Field, "field_not_clearable", e.Error()
}

// applyClears sets each named field to NULL, and refuses a name this store
// cannot clear. A field the map does not hold is either not nullable or not
// clearable through this path, and either way the honest answer is to say so
// rather than accept the instruction and drop it.
func applyClears(p *storekit.Patch, fields []string, columns map[string]clearable) error {
	for _, field := range fields {
		target, clearableHere := columns[field]
		if !clearableHere {
			return &NotClearableError{Field: field}
		}
		p.Set(target.column, target.current, nil)
	}
	return nil
}

// clearablePersonColumns maps the wire fields a person restore may set to NULL
// onto the column holding each, with the row's current value for the audit
// image. The column names are literals here and never come from a caller, so
// nothing caller-supplied reaches the UPDATE text.
//
// A field absent from this map cannot be cleared, and the reversal path refuses
// rather than reporting a success it did not have.
func clearablePersonColumns(current crmcontracts.Person) map[string]clearable {
	return map[string]clearable{
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
func clearableOrganizationColumns(current crmcontracts.Organization) map[string]clearable {
	return map[string]clearable{
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
func clearableLeadColumns(current crmcontracts.Lead) map[string]clearable {
	return map[string]clearable{
		"title":             {"title", current.Title},
		"company_name":      {leadCompanyColumn, current.CompanyName},
		"candidate_org_key": {"candidate_org_key", current.CandidateOrgKey},
		"project_id":        {"project_id", current.ProjectId},
		"owner_id":          {ownerIDColumn, current.OwnerId},
	}
}
