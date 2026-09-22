// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// A masked column refused as a sort key. Ordering by a value is reading it, so
// a page ordered by figures the caller may not see discloses them through the
// order — and every list that offers a maskable column as an order owes the
// same refusal in the same words, which is why it is spelled here and not in
// each module that happens to publish one.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RefuseMaskedSort refuses a sort naming a column this caller's role masks on
// any row of the object. maskable reports whether the object can withhold the
// field at all, so a sort key no mask can reach costs nothing to admit.
//
// The list's own vocabulary has already refused a field it cannot order by, so
// a name reaching here is one this caller may ask for and only the mask denies.
func RefuseMaskedSort(ctx context.Context, object string, sort *string, maskable func(field string) bool) error {
	if sort == nil || *sort == "" {
		return nil
	}
	field := strings.TrimPrefix(strings.TrimSpace(*sort), "-")
	if !maskable(field) {
		return nil
	}
	masked, err := MasksAnyRowOf(ctx, object, field)
	if err != nil {
		return err
	}
	if !masked {
		return nil
	}
	return &values.ParseError{
		Field: "sort", Code: CodeFieldMasked,
		Message: "sort by " + field + " is not available: your role does not read it on every " + object,
	}
}
