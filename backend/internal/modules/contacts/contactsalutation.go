// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

const fieldSalutation = "salutation"

func checkSalutation(value *crmcontracts.Salutation) error {
	if value == nil || value.Valid() {
		return nil
	}
	return httperr.Validation(fieldSalutation, codeInvalidValue, "Choose mr, ms or none, or clear the salutation")
}

func patchSalutation(p *storekit.Patch, current, value *crmcontracts.Salutation) error {
	if err := checkSalutation(value); err != nil {
		return err
	}
	if value != nil {
		p.Set(fieldSalutation, current, string(*value))
	}
	return nil
}

// contactCreateImage keeps an explicitly supplied salutation in the audit.
func contactCreateImage(in CreateContactInput) map[string]any {
	image := map[string]any{"full_name": in.FullName}
	if in.Salutation != nil {
		image[fieldSalutation] = *in.Salutation
	}
	return image
}

// salutationListClause keeps an unset choice distinct from an explicit none.
// Unknown filter values match no rows, as the other word filters do.
func salutationListClause(table string, value *string, arg func(any) int) string {
	if value == nil {
		return ""
	}
	if *value == "unset" {
		return table + ".salutation IS NULL"
	}
	return storekit.SQLf("%s.salutation = $%d", table, arg(*value))
}

// SubmittedSalutation maps a choice explicitly sent by a form or an import.
// Unknown nonempty choices mean none; an absent choice stays unset.
func SubmittedSalutation(value string) *crmcontracts.Salutation {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if value == "" {
		return nil
	}
	choice := crmcontracts.SalutationNone
	switch value {
	case "herr", "mr", "hr":
		choice = crmcontracts.SalutationMr
	case "frau", "ms", "mrs", "fr":
		choice = crmcontracts.SalutationMs
	}
	return &choice
}
