// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// fullNameDisagreesCode is the field-fault code for a full name that names
// someone other than the first and last name sent with it.
const fullNameDisagreesCode = "full_name_disagrees"

// refuseDisagreeingNames refuses a full name that is neither order of the first
// and last name beside it. A record must not name one human in its title and
// another in its fields. It asks only when all three are given.
func refuseDisagreeingNames(full string, first, last *string) error {
	if first == nil || last == nil {
		return nil
	}
	given, family, name := foldedNameKey(*first), foldedNameKey(*last), foldedNameKey(full)
	if given == "" || family == "" || name == "" {
		return nil
	}
	if name == given+" "+family || name == family+" "+given {
		return nil
	}
	return &values.ParseError{
		Field: "full_name", Code: fullNameDisagreesCode,
		Message: fmt.Sprintf("full_name %q does not match first_name and last_name %q; send names that match, or leave full_name out",
			strings.TrimSpace(full), strings.TrimSpace(*first)+" "+strings.TrimSpace(*last)),
	}
}
