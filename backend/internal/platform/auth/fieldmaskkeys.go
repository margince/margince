// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// A masked column refused as a sort or filter key. Ordering by a value is
// reading it and so is narrowing by it: a page ordered by figures the caller
// may not see discloses them through the order, and one narrowed by a reference
// they may not read discloses it through which rows come back. Every list
// offering a maskable column owes the same refusal in the same words, which is
// why it is spelled here and not in each module that happens to publish one.

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
	return refuseMaskedKey(ctx, object, "sort", field, "sort")
}

// RefuseMaskedFilter refuses a filter narrowing by a column this caller's role
// masks on any row of the object. The refusal is the whole answer: a page
// narrowed for real hands the column over through its membership, and an empty
// one would be just as safe while teaching the caller the value.
//
// parameter is what the caller sent, field the column it reads. They coincide
// wherever a list names a filter after its column; a filter named for the
// question it asks — partner_sourced, over partner_company_id — narrows a
// masked column all the same, so the refusal points at the parameter while
// naming the column that closed it.
func RefuseMaskedFilter(ctx context.Context, object, parameter, field string) error {
	return refuseMaskedKey(ctx, object, parameter, field, "filter")
}

// refuseMaskedKey is the refusal both keys owe: one question of the role and
// one wording, so a client that handles it on a sort handles it on a filter.
func refuseMaskedKey(ctx context.Context, object, parameter, field, verb string) error {
	masked, err := MasksAnyRowOf(ctx, object, field)
	if err != nil || !masked {
		return err
	}
	return &values.ParseError{
		Field: parameter, Code: CodeFieldMasked,
		Message: verb + " by " + field + " is not available: your role does not read it on every " + object,
	}
}
