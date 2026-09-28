// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Naming a withheld field as a history filter is refused rather than answered.
// The trail's mask is the live read's mask — hiding history and value is one
// motion — and withheldHistoryOf beside the diff resolves it.

import (
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// refuseMaskedFieldFilter refuses a field filter naming a field this reader's
// masks withhold. An empty page would be just as safe and would teach the
// reader the value never changed.
func refuseMaskedFieldFilter(field *string, mask entityFieldMask, entityType string) error {
	if field == nil {
		return nil
	}
	if _, withheld := mask[*field]; !withheld {
		return nil
	}
	return &values.ParseError{
		Field: "field", Code: auth.CodeFieldMasked,
		Message: "the history of " + *field + " is not available: your role does not read it on this " + entityType,
	}
}
