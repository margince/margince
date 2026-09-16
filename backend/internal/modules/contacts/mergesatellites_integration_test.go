// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a merge does with a reader's judgements and with work still open.
//
// Every one of these rows used to stay on the merged-away contact. Nothing
// failed: the row is still there, naming an id no read returns, and the product
// simply behaves as though the decision was never made — a dismissed moment
// comes back, a hand-off nobody can see is a prospect dropped.
//
// This needs a database because the collisions are constraint-shaped: both
// halves may carry a dismissal for the same reader and the same claim, and
// contact_signature_enrich_state is one row per contact, so the carry has to
// decide which survives rather than insert a second.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) execAs(ctx context.Context, t *testing.T, sql string, args ...any) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *dedupeEnv) countAs(ctx context.Context, t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, sql, args...).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// A DISMISSAL IS AN ANSWER, and a merge must not un-give it.
//
// The reader looked at the moment and said no. Left on the retired half that
// answer is invisible to every read of the survivor, so the moment returns, the
// rep dismisses it a second time, and nothing anywhere says why it came back.
//
// The collision is the other half of the case: both records may carry the same
// reader's dismissal of the same claim, and the survivor's stands — it was made
// with the record that remains in front of them.
func TestAMergeCarriesTheDismissalsAReaderAlreadyGave(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@dismiss.test", "Keeper", "dismiss.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@dismiss.test", "Dupe", "dismiss.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}
	reader := ids.NewV7()
	e.execAs(ctx, t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Reader')`,
		reader, "reader@dismiss.test")

	// One the retired half alone carries: it moves.
	e.execAs(ctx, t, `
		INSERT INTO contact_moment_dismissal (user_id, contact_id, claim_key, evidence_fingerprint)
		VALUES ($1, $2, 'moved_on', 'fp-1')`, reader, retired.ContactID)
	// One BOTH carry for the same reader and claim: the survivor's stands.
	e.execAs(ctx, t, `
		INSERT INTO contact_moment_dismissal (user_id, contact_id, claim_key, evidence_fingerprint)
		VALUES ($1, $2, 'both_had_it', 'from-the-retired'), ($1, $3, 'both_had_it', 'from-the-survivor')`,
		reader, retired.ContactID, survivor.ContactID)
	e.execAs(ctx, t, `
		INSERT INTO relationship_nudge_dismissal (contact_id, reader_id, dismissed_until, set_by)
		VALUES ($1, $2, now() + interval '30 days', 'human:x')`, retired.ContactID, reader)

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM contact_moment_dismissal WHERE contact_id = $1`, retired.ContactID); n != 0 {
		t.Errorf("%d dismissal(s) still name the merged-away contact, want 0 — a row there is an "+
			"answer nobody can read, and the moment comes back on the survivor", n)
	}
	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM contact_moment_dismissal WHERE contact_id = $1`, survivor.ContactID); n != 2 {
		t.Errorf("the survivor holds %d dismissal(s), want 2 — the one only the retired half had, "+
			"and the one they both had, once", n)
	}
	var fingerprint string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT evidence_fingerprint FROM contact_moment_dismissal
			 WHERE contact_id = $1 AND claim_key = 'both_had_it'`, survivor.ContactID).Scan(&fingerprint)
	}); err != nil {
		t.Fatal(err)
	}
	if fingerprint != "from-the-survivor" {
		t.Errorf("the collided dismissal reads %q, want the survivor's own — theirs was made about "+
			"the record that remains", fingerprint)
	}
	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM relationship_nudge_dismissal WHERE contact_id = $1`, survivor.ContactID); n != 1 {
		t.Errorf("the survivor holds %d nudge dismissal(s), want 1 — a nudge the reader put down "+
			"comes straight back when the row stays on the merged-away half", n)
	}
}

// WORK STILL OPEN follows the survivor, and the cached sentence about the
// record goes.
//
// A claim is a commitment somebody has to answer and a hand-off is a prospect
// waiting for an AE; both are invisible on the retired id. A brief is neither —
// it is a cached summary, and after a merge BOTH copies describe a record that
// no longer exists in the shape they were written about.
func TestAMergeCarriesOpenWorkAndDropsTheStaleBrief(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@open.test", "Keeper", "open.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@open.test", "Dupe", "open.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}
	reader := ids.NewV7()
	e.execAs(ctx, t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Reader')`,
		reader, "reader@open.test")
	activity := e.seedCapturedMail(ctx, t, "elsewhere@open.test", "from")

	e.execAs(ctx, t, `
		INSERT INTO conversation_claim
		  (contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by)
		VALUES ($1, 'commitment_ours', 'send the pricing', $2, 'we will send pricing', 'fp', 'manual', 'human:x')`,
		retired.ContactID, activity)
	e.execAs(ctx, t, `
		INSERT INTO contact_signature_enrich_state (contact_id, activity_id, last_activity_at)
		VALUES ($1, $2, now())`, retired.ContactID, activity)
	e.execAs(ctx, t, `
		INSERT INTO contact_brief (user_id, contact_id, fingerprint, payload, generated_by)
		VALUES ($1, $2, 'fp-retired', '{}'::jsonb, 'deterministic'),
		       ($1, $3, 'fp-survivor', '{}'::jsonb, 'deterministic')`,
		reader, retired.ContactID, survivor.ContactID)

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM conversation_claim WHERE contact_id = $1`, survivor.ContactID); n != 1 {
		t.Errorf("the survivor holds %d open claim(s), want 1 — a commitment left on the "+
			"merged-away record is one the workspace made and can no longer answer", n)
	}
	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id = $1`, retired.ContactID); n != 0 {
		t.Errorf("%d enrichment attempt(s) still name the merged-away contact, want 0", n)
	}
	if n := e.countAs(ctx, t,
		`SELECT count(*) FROM contact_brief WHERE contact_id IN ($1, $2)`,
		retired.ContactID, survivor.ContactID); n != 0 {
		t.Errorf("%d brief(s) survived the merge, want 0 — both were written about a record that "+
			"no longer stands in the shape they describe", n)
	}
}
