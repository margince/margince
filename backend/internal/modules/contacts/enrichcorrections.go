// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which fields a human has already ruled on, asked inside the write's own
// transaction.

import (
	"context"
	"errors"
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
func correctedFields(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []SignatureField) (map[string]bool, error) {
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
		WHERE subject_type = 'contact' AND subject_id = $1
		  AND claim_kind = 'profile_field' AND verdict = 'corrected'
		  AND claim_key = ANY($2)`, contactID, keys)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading this contact's corrections: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("contacts: reading a correction: %w", err)
		}
		out[byKey[key]] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading this contact's corrections: %w", err)
	}
	return out, nil
}

// correctedProfileFieldNames answers which of these fields a human has ruled
// on, for a caller that holds field names and no ledger keys.
//
// The key is spelt in SQL from the claim path, as the ledger builds it, and
// TestTheRestoreMatchesTheLedgersOwnClaimKey holds the two spellings together.
func correctedProfileFieldNames(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(fields) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT f FROM unnest($2::text[]) AS f
		 WHERE EXISTS (
			SELECT 1 FROM ai_feedback
			 WHERE subject_type = 'contact' AND subject_id = $1
			   AND claim_kind = 'profile_field' AND verdict = 'corrected'
			   AND claim_key = encode(sha256(('profile_field:' || f)::bytea), 'hex'))`,
		contactID, fields)
	if err != nil {
		return nil, fmt.Errorf("contacts: looking for corrections on this contact's fields: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var field string
		if err := rows.Scan(&field); err != nil {
			return nil, fmt.Errorf("contacts: reading a corrected field: %w", err)
		}
		out[field] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: looking for corrections on this contact's fields: %w", err)
	}
	return out, nil
}

// verdictCorrected is the ledger's word for a human's correction.
const verdictCorrected = "corrected"

// errNoProfileVerdictReader refuses a write that must ask whether a correction
// still stands, on a store that was built without the reader that answers it.
var errNoProfileVerdictReader = errors.New("contacts: this store was built without the profile verdict reader")

// standingCorrections answers which of these fields carry a correction that
// still describes the value the record holds.
//
// The ledger rows are only candidates. Whether one applies is the verdict
// module's ruling, through the reader a merge preview uses. A correction of a
// value since replaced then no longer stops a newer statement.
func (s *Store) standingCorrections(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []string) (map[string]bool, error) {
	candidates, err := correctedProfileFieldNames(ctx, tx, contactID, fields)
	if err != nil || len(candidates) == 0 {
		return candidates, err
	}
	if s.profileVerdictReader == nil {
		return nil, errNoProfileVerdictReader
	}
	named := make([]string, 0, len(candidates))
	for field := range candidates {
		named = append(named, field)
	}
	assertions, err := heldProfileValues(ctx, tx, contactID, named)
	if err != nil {
		return nil, err
	}
	ruled, err := s.profileVerdictReader(ctx, tx, contactID.UUID, assertions)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, value := range ruled {
		if value.Verdict != nil && value.Verdict.Verdict == verdictCorrected {
			out[value.Field] = true
		}
	}
	return out, nil
}

// heldProfileValues reads the stored rows of these fields as the verdict
// reader takes them: the value and when it took its current form.
func heldProfileValues(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []string) ([]MergeProfileAssertion, error) {
	rows, err := tx.Query(ctx, `
		SELECT field, value_key, value, updated_at FROM contact_profile_field
		 WHERE contact_id = $1 AND field = ANY($2)`, contactID, fields)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the values a correction may describe: %w", err)
	}
	defer rows.Close()
	var out []MergeProfileAssertion
	for rows.Next() {
		var a MergeProfileAssertion
		if err := rows.Scan(&a.Field, &a.ValueKey, &a.Value, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("contacts: reading a value a correction may describe: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading the values a correction may describe: %w", err)
	}
	return out, nil
}
