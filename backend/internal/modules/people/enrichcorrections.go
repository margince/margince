// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Which fields a human has already ruled on, asked inside the write's own
// transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// correctedFields answers which of these fields a human has already ruled on,
// read inside the apply's own transaction.
//
// The caller carries the claim KEY for each field, because the ledger stores a
// hash of the claim path and only the module that owns the ledger can compute
// one. Matching on a key this package built itself would be a second spelling
// of that path, and since the stored value is a hash the mismatch would show up
// as nothing at all: every correction silently unseen, every field overwritten.
func correctedFields(ctx context.Context, tx pgx.Tx, personID ids.PersonID, fields []SignatureField) (map[string]bool, error) {
	byKey := make(map[string]string, len(fields))
	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.ClaimKey == "" {
			continue
		}
		byKey[f.ClaimKey] = f.Name
		keys = append(keys, f.ClaimKey)
	}
	if len(keys) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT claim_key FROM ai_feedback
		WHERE subject_type = 'person' AND subject_id = $1
		  AND claim_kind = 'profile_field' AND verdict = 'corrected'
		  AND claim_key = ANY($2)`, personID, keys)
	if err != nil {
		return nil, fmt.Errorf("people: reading this person's corrections: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("people: reading a correction: %w", err)
		}
		out[byKey[key]] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading this person's corrections: %w", err)
	}
	return out, nil
}
