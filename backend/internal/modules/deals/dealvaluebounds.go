// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strconv"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func dealDescriptionWithin(description *string) error {
	if description == nil {
		return nil
	}
	return httperr.RequireWithin("description", *description, values.TextDocumentCharacters)
}

func dealAmountUpperBound(field string, amount *int64) error {
	if amount == nil || *amount <= values.MaxExactInteger {
		return nil
	}
	return httperr.Validation(field, "out_of_range", "the amount exceeds the largest exact JSON integer").WithParams(map[string]string{"min": "0", "max": strconv.FormatInt(values.MaxExactInteger, 10)})
}
