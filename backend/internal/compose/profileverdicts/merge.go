// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package profileverdicts connects profile verdict visibility to native merge readers.
package profileverdicts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeReader keeps merge previews and the card import under the same verdict
// ruling as profile reads.
func MergeReader(db *database.DB) contacts.ProfileVerdictReader {
	store := ai.NewFeedbackStore(db)
	return func(ctx context.Context, tx pgx.Tx, contact ids.UUID, assertions []contacts.MergeProfileAssertion) ([]contacts.MergeProfileValue, error) {
		verdicts, err := store.VerdictsForTx(ctx, tx, "contact", contact)
		if err != nil {
			return nil, err
		}
		values := make([]contacts.MergeProfileValue, 0, len(assertions))
		for _, assertion := range assertions {
			value := contacts.MergeProfileValue{MergeProfileAssertion: assertion}
			key := ai.VerdictLookupKey(ai.ClaimProfileField, ai.ClaimKey(ai.ProfileFieldClaimPath(assertion.Field)))
			if verdict, found := verdicts[key]; found {
				if decision, applies := verdict.AsOf(assertion.Value, assertion.UpdatedAt); applies {
					value.Verdict = &contacts.MergeProfileVerdict{Verdict: decision.Verdict, Note: decision.Note, CorrectedValue: decision.CorrectedValue}
				}
			}
			values = append(values, value)
		}
		return values, nil
	}
}
