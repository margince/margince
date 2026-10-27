// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"unicode/utf8"
)

// CompanyDescriptionTooLongError names the field and the bar. The
// company_description_length CHECK refuses the same write, but its answer names
// no field, so a caller could not tell which value to shorten.
type CompanyDescriptionTooLongError struct{ Runes int }

func (e *CompanyDescriptionTooLongError) Error() string {
	return fmt.Sprintf("contacts: a company description is at most %d characters, this one is %d",
		CompanyDescriptionMax, e.Runes)
}

// FieldFault names the description for every surface, the MCP tools included.
func (e *CompanyDescriptionTooLongError) FieldFault() (field, code, message string) {
	return columnDescription, "too_long", e.Error()
}

// checkCompanyDescription is the one place create and update refuse a
// description the column would refuse.
func checkCompanyDescription(description *string) error {
	if description == nil {
		return nil
	}
	if runes := utf8.RuneCountInString(*description); runes > CompanyDescriptionMax {
		return &CompanyDescriptionTooLongError{Runes: runes}
	}
	return nil
}
