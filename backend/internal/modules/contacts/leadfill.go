// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

// emptyFieldPatch is the sparse patch: a captured value, cut to its column's
// limit, for every field the lead lacks, and nothing for the rest.
//
// Email is absent. It is the lead's dedupe key and the reason this collision
// was found, so the incumbent always has one. A captured address could only be
// a second address, which is an employment question and not a blank to fill.
func emptyFieldPatch(current crmcontracts.Lead, in CapturedLeadFields) UpdateLeadInput {
	var patch UpdateLeadInput
	if name := textbounds.Fit("public.lead.full_name", strings.TrimSpace(in.FullName)); name != "" && blank(current.FullName) {
		patch.FullName = &name
	}
	if company := textbounds.Fit("public.lead.company_name", in.CompanyName); company != "" && blank(current.CompanyName) {
		patch.CompanyName = &company
	}
	if title := textbounds.Fit("public.lead.title", in.Title); title != "" && blank(current.Title) {
		patch.Title = &title
	}
	return patch
}

func blank(value *string) bool { return value == nil || *value == "" }
