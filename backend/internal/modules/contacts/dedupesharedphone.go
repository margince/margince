// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sharedContactPhone(ctx context.Context, tx pgx.Tx, left, right ids.UUID) (string, bool, error) {
	var shared string
	err := tx.QueryRow(ctx, `SELECT l.phone FROM contact_phone l
		JOIN contact_phone r ON r.phone = l.phone
		WHERE l.contact_id = @left AND r.contact_id = @right
		  AND l.archived_at IS NULL AND r.archived_at IS NULL
		ORDER BY l.phone LIMIT 1`, pgx.StrictNamedArgs{"left": left, "right": right}).Scan(&shared)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("contacts: reading the shared phone number: %w", err)
	}
	return shared, true, nil
}

func refreshContactPhoneEvidence(ctx context.Context, tx pgx.Tx, pair openPair, left, right contactSide, score float64) error {
	evidence := nearMatchEvidence(fieldFullName, left.name, right.name, score)
	if score < dedupeReviewThreshold {
		if contactNamesCollide(left.name, right.name) {
			evidence = nameCollisionEvidence(left.name, right.name, score)
		} else if candidate, incumbent, variant := addressVariantPair(left.emails, right.emails); variant {
			evidence = addressVariantEvidence(candidate, incumbent)
			score = 0
		} else if linkedInProfilesMatch(left.linkedIn, right.linkedIn) {
			evidence = linkedInEvidence(left.linkedIn, right.linkedIn)
			score = 0
		}
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("contacts: encoding the duplicate pair's current evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE dedupe_candidate
		SET confidence = @score, evidence = @evidence, version = version + 1
		WHERE id = @id`, pgx.StrictNamedArgs{evidenceScoreKey: score, "evidence": encoded, "id": pair.id}); err != nil {
		return fmt.Errorf("contacts: updating the duplicate pair's current evidence: %w", err)
	}
	_, err = storekit.Audit(ctx, tx, "update", auditEntityDedupe, pair.id,
		map[string]any{auditKeyConfidence: pair.confidence},
		map[string]any{auditKeyConfidence: score, auditKeyReason: "shared_phone_removed", "evidence": evidence})
	return err
}

// Lane conflicts and unfamiliar evidence stand for a human to decide.
const phoneCollisionEvidenceSQL = `CASE WHEN jsonb_typeof(d.evidence) = 'array' THEN
	jsonb_array_length(d.evidence) > 0 AND NOT EXISTS (
		SELECT 1 FROM jsonb_array_elements(d.evidence) e
		WHERE e->>'field' IS DISTINCT FROM '` + fieldPhone + `'
		   OR e->>'signal' IS DISTINCT FROM '` + evidenceSignalCollide + `')
	ELSE false END`
