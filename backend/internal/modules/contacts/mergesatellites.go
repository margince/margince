// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The satellites a merge used to leave behind.
//
// Split from mergerelink.go because that file reached its ceiling, and because
// this half answers a different question. Every relink there says where a row
// LIVES now. These say what somebody DECIDED and what is still OPEN — and a row
// of either kind stranded on the merged-away contact is worse than invisible,
// because it silently changes what the product does next.
//
// Nothing here is reached by file name. The lifecycle census
// (gates/satellite_lifecycle_test.go) walks what relinkContactReferences can
// CALL, so a satellite joins the merge by being written somewhere under it,
// wherever that happens to live.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkReaderJudgements moves what a READER decided about this contact, and
// drops what was merely computed about them.
//
// The difference is the whole of it. A DISMISSAL is somebody's answer — they
// looked at a moment or a nudge and said no — and an answer left on the retired
// half is an answer silently un-given: the moment returns on the survivor, the
// rep dismisses it a second time, and nothing says why it came back. A BRIEF is
// not an answer. It is a cached sentence about a record that no longer stands
// alone, so BOTH halves' copies go and the next read writes one about the
// record that does — the survivor's is stale too, having been written before it
// held the merged-away half's addresses and history.
//
// SURVIVOR WINS every collision, the rule the primary-slot demotions already
// take: their judgement is about the record that remains, and it was made with
// that record in front of them.
func relinkReaderJudgements(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact_moment_dismissal (user_id, contact_id, claim_key, evidence_fingerprint, dismissed_at)
		SELECT user_id, $2, claim_key, evidence_fingerprint, dismissed_at
		  FROM contact_moment_dismissal WHERE contact_id = $1
		ON CONFLICT (user_id, contact_id, claim_key) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry the moments a reader had already dismissed: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_moment_dismissal WHERE contact_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away dismissals: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO relationship_nudge_dismissal (contact_id, reader_id, dismissed_until, set_by, set_at)
		SELECT $2, reader_id, dismissed_until, set_by, set_at
		  FROM relationship_nudge_dismissal WHERE contact_id = $1
		ON CONFLICT (contact_id, reader_id) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry the nudges a reader had already put down: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM relationship_nudge_dismissal WHERE contact_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away nudge dismissals: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_brief WHERE contact_id IN ($1, $2)`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the briefs the merge made stale: %w", err)
	}
	return nil
}

// relinkWorkInFlight moves what is still open against the merged-away contact.
//
// A claim somebody has to answer, a hand-off waiting for an AE to accept it,
// and the signature enrichment's note of what it last tried. Each hangs off the
// retired id and is invisible to every read of the survivor: a hand-off nobody
// can see is a prospect dropped, and a claim nobody can see is a commitment the
// workspace made and can no longer answer.
//
// The enrich state keeps the SURVIVOR'S row rather than the source's. It is one
// row per contact saying what the last attempt was, and the attempt that
// describes the record still standing is the survivor's.
func relinkWorkInFlight(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE conversation_claim SET contact_id = $2 WHERE contact_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("relink the claims still open against this contact: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sdr_handoff SET contact_id = $2 WHERE contact_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("relink the hand-offs naming this contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact_signature_enrich_state (contact_id, activity_id, last_activity_at, attempted_at)
		SELECT $2, activity_id, last_activity_at, attempted_at
		  FROM contact_signature_enrich_state WHERE contact_id = $1
		ON CONFLICT (contact_id) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry the signature-enrichment attempt: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_signature_enrich_state WHERE contact_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away enrichment attempt: %w", err)
	}
	return nil
}
