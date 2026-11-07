// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The duplicate-pair evidence snapshot against real rows.
//
// The table-set census in backend/gates/contactscrub_test.go can prove both
// privacy acts WRITE dedupe_candidate. What it cannot see is whether the
// statement reaches the row: the candidate is keyed on six nullable subject
// columns under a shape constraint, so a pair filed as a LEAD reached by a
// contact-keyed predicate reads exactly like one that works and empties
// nothing.
//
// So all three anonymize paths run here against a seeded pair of each kind, and
// each is asked about BOTH kinds. Which pairs an act must empty differs — the
// sweep's contact arm never meets a lead and its lead arm never meets a contact
// — and one expectation shared across the three would assert only what the
// weakest of them does.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dedupeSubjects is one seeded subject: the contact, the lead they were
// promoted from, and the addresses the erasure resolves them by.
type dedupeSubjects struct {
	contact ids.ContactID
	lead    ids.UUID
	emails  []string
}

// The three acts that anonymize a subject in place, with the pair kinds each
// one has to reach.
var dedupeEvidenceActs = map[string]struct {
	run            func(context.Context, pgx.Tx, dedupeSubjects) error
	contact, leads bool
}{
	"an Art. 17 request": {
		run: func(ctx context.Context, tx pgx.Tx, s dedupeSubjects) error {
			// No channel accounts: the account list reaches only the
			// participant scrub, which has its own erasure test.
			_, err := anonymizeSubjectRows(ctx, tx, s.contact, s.emails, nil, "test")
			return err
		},
		contact: true, leads: true,
	},
	"the retention sweep's contact arm": {
		run: func(ctx context.Context, tx pgx.Tx, s dedupeSubjects) error {
			return anonymizeContactRecord(ctx, tx, s.contact.UUID, &Eraser{})
		},
		contact: true,
	},
	"the retention sweep's lead arm": {
		run: func(ctx context.Context, tx pgx.Tx, s dedupeSubjects) error {
			return (&RetentionService{}).anonymizeLead(ctx, tx, s.lead)
		},
		leads: true,
	},
}

func TestAnonymizingASubjectEmptiesTheirDuplicatePairEvidence(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	for name, act := range dedupeEvidenceActs {
		t.Run(name, func(t *testing.T) {
			for _, state := range storedDedupeDispositions(ctx, t) {
				for _, archived := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/archived=%t", state, archived), func(t *testing.T) {
						tx := subjectColumnsTx(ctx, t)
						subjects, contactPair, leadPair := seedDedupePairs(ctx, t, tx)
						mustExec(ctx, t, tx, `UPDATE dedupe_candidate
							SET disposition = @state,
							    disposed_at = CASE WHEN @state = 'open' THEN NULL ELSE timestamptz '2000-01-01 00:00:00+00' END,
							    disposed_by = CASE WHEN @state = 'open' THEN NULL ELSE split_part(captured_by, ':', 2)::uuid END,
							    archived_at = CASE WHEN @archived THEN timestamptz '2000-01-01 00:00:00+00' ELSE NULL END
							WHERE id = ANY(@pairs)`, pgx.StrictNamedArgs{
							"state": state, "archived": archived, "pairs": []ids.UUID{contactPair, leadPair},
						})
						contactBefore := readDedupeScrubState(ctx, t, tx, contactPair)
						leadBefore := readDedupeScrubState(ctx, t, tx, leadPair)
						if err := act.run(ctx, tx, subjects); err != nil {
							t.Fatalf("anonymizing: %v", err)
						}
						assertDedupeScrubState(t, contactBefore, readDedupeScrubState(ctx, t, tx, contactPair), act.contact, state)
						assertDedupeScrubState(t, leadBefore, readDedupeScrubState(ctx, t, tx, leadPair), act.leads, state)
						var contacts, leads []ids.UUID
						if act.contact {
							contacts = []ids.UUID{subjects.contact.UUID}
						}
						if act.leads {
							leads = []ids.UUID{subjects.lead}
						}
						if err := scrubDedupeEvidence(ctx, tx, contacts, leads); err != nil {
							t.Fatal(err)
						}
						assertDedupeScrubState(t, contactBefore, readDedupeScrubState(ctx, t, tx, contactPair), act.contact, state)
						assertDedupeScrubState(t, leadBefore, readDedupeScrubState(ctx, t, tx, leadPair), act.leads, state)
						for pair, targeted := range map[ids.UUID]bool{contactPair: act.contact, leadPair: act.leads} {
							var count int
							if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log
                                WHERE entity_type='dedupe_candidate' AND entity_id=$1 AND action='archive'
                                AND after = '{"archived":true,"reason":"subject_erased"}'::jsonb`, pair).Scan(&count); err != nil {
								t.Fatal(err)
							}
							want := 0
							if targeted && state == "open" && !archived {
								want = 1
							}
							if count != want {
								t.Errorf("closure audits=%d, want %d", count, want)
							}
						}

					})
				}
			}
		})
	}
}

func storedDedupeDispositions(ctx context.Context, t *testing.T) []string {
	t.Helper()
	tx := subjectColumnsTx(ctx, t)
	var check string
	if err := tx.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'dedupe_candidate'::regclass AND conname = 'dedupe_candidate_disposition_check'`).Scan(&check); err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(check, -1)
	if len(matches) < 3 {
		t.Fatalf("dedupe disposition census is too short: %s", check)
	}
	states := make([]string, len(matches))
	for i, match := range matches {
		states[i] = match[1]
	}
	return states
}

type dedupeScrubState struct {
	metadata json.RawMessage
	evidence json.RawMessage
	archived string
	version  int64
}

func readDedupeScrubState(ctx context.Context, t *testing.T, tx pgx.Tx, id ids.UUID) dedupeScrubState {
	t.Helper()
	var state dedupeScrubState
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(d) - ARRAY['evidence', 'archived_at', 'updated_at', 'version'],
		evidence, coalesce(archived_at::text, ''), version FROM dedupe_candidate d WHERE id = @id`,
		pgx.StrictNamedArgs{"id": id}).Scan(&state.metadata, &state.evidence, &state.archived, &state.version); err != nil {
		t.Fatal(err)
	}
	return state
}

func assertDedupeScrubState(t *testing.T, before, after dedupeScrubState, scrubbed bool, disposition string) {
	t.Helper()
	if !bytes.Equal(before.metadata, after.metadata) {
		t.Errorf("duplicate decision metadata changed: before=%s after=%s", before.metadata, after.metadata)
	}
	if !scrubbed {
		if !bytes.Equal(before.evidence, after.evidence) || before.version != after.version || before.archived != after.archived {
			t.Errorf("another subject's duplicate match changed: before=%+v after=%+v", before, after)
		}
		return
	}
	if string(after.evidence) != emptyDedupeEvidence || after.version != before.version+1 {
		t.Errorf("duplicate evidence was not cleared once: before=%+v after=%+v", before, after)
	}
	if disposition == "open" && after.archived == "" {
		t.Error("a pending duplicate match was not closed")
	}
	if (disposition != "open" || before.archived != "") && before.archived != after.archived {
		t.Errorf("an existing decision's archive state changed: before=%q after=%q", before.archived, after.archived)
	}
}

// seedDedupePairs files one contact pair and one lead pair naming the subject,
// each carrying the identifiers the detector actually stores: the name it
// matched on and the address only one side held.
//
// The twin on each side is a SECOND live record, because a pair is a claim
// about two of them — and the ordered constraint refuses a self-pair outright.
func seedDedupePairs(ctx context.Context, t *testing.T, tx pgx.Tx) (dedupeSubjects, ids.UUID, ids.UUID) {
	t.Helper()
	ws, user := ids.NewV7(), ids.NewV7()
	contact, twin := ids.New[ids.ContactKind](), ids.New[ids.ContactKind]()
	lead, leadTwin := ids.NewV7(), ids.NewV7()
	const subjectEmail = "hedda@anon.test"

	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	for _, id := range []ids.ContactID{contact, twin} {
		mustExec(ctx, t, tx,
			`INSERT INTO contact (id, full_name, source, captured_by)
			 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, id, user)
	}
	mustExec(ctx, t, tx,
		`INSERT INTO contact_email (contact_id, email, source, captured_by)
		 VALUES ($1, $2, 'manual', 'user:'||$3::text)`, contact, subjectEmail, user)
	// The twin the Art. 17 cascade reaches through promoted_contact_id, and a
	// second lead for it to be a pair with.
	mustExec(ctx, t, tx,
		`INSERT INTO lead (id, full_name, source, captured_by, promoted_contact_id)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text, $3)`, lead, user, contact)
	mustExec(ctx, t, tx,
		`INSERT INTO lead (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, leadTwin, user)

	return dedupeSubjects{contact: contact, lead: lead, emails: []string{subjectEmail}},
		fileDedupePair(ctx, t, tx, "contact", contact.UUID, twin.UUID, subjectEmail, user),
		fileDedupePair(ctx, t, tx, "lead", lead, leadTwin, subjectEmail, user)
}

// fileDedupePair orders the pair's two ends before inserting, because
// dedupe_candidate_ordered refuses the other spelling and a fixture that
// happened to seed them the right way round half the time would fail on the
// id draw rather than on the code.
func fileDedupePair(
	ctx context.Context, t *testing.T, tx pgx.Tx,
	entity string, a, b ids.UUID, email string, user ids.UUID,
) ids.UUID {
	t.Helper()
	left, right := a, b
	if right.String() < left.String() {
		left, right = right, left
	}
	evidence, err := json.Marshal([]map[string]any{
		{
			"field": "full_name", "left_value": "Hedda Subject", "right_value": "Hedda Subject",
			"signal": "collide", "score": 0.91, "future_detail": map[string]string{"private_note": "Hedda Subject"},
		},
		{"field": "email", "left_value": email, "right_value": nil, "signal": "one_sided"},
	})
	if err != nil {
		t.Fatalf("building the evidence snapshot: %v", err)
	}
	id := ids.NewV7()
	column := map[string]string{
		"contact": `left_contact_id, right_contact_id`,
		"lead":    `left_lead_id, right_lead_id`,
	}[entity]
	mustExec(ctx, t, tx, `
		INSERT INTO dedupe_candidate (id, entity_type, `+column+`, confidence, evidence, source, captured_by)
		VALUES (@id, @entity, @left, @right, 0.91, @evidence, 'manual', 'user:'||@user::text)`,
		pgx.StrictNamedArgs{"id": id, "entity": entity, "left": left, "right": right, "evidence": evidence, "user": user})
	return id
}

// An erasure step that CANNOT write says so.
//
// Each of these ends in a table the cascade does not own, and is the last thing
// standing between a subject and a copy of their name. A swallowed failure here
// is the worst shape the erasure can take: the transaction commits, the
// certificate says destroyed, and the row is still readable.
//
// An aborted transaction is what that looks like from inside the cascade —
// every later statement is refused — and it is the one fault a test can stage
// without taking the table away from every other suite on the lane.
func TestAScrubThatCannotWriteReportsItRatherThanCommitting(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.New[ids.ContactKind]()

	// The deliberate fault. Its own failure is the fixture, so it is asserted:
	// a statement that succeeded would leave the transaction writable and every
	// case below passing for the wrong reason.
	if _, err := tx.Exec(ctx, `SELECT 1 / 0`); err == nil {
		t.Fatal("the fixture's deliberate division by zero succeeded; nothing below is aborted")
	}

	for name, scrub := range map[string]func() error{
		"the duplicate-pair evidence": func() error {
			return scrubDedupeEvidence(ctx, tx, []ids.UUID{subject.UUID}, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := scrub(); err == nil {
				t.Errorf("clearing %s reported success on a transaction that can no longer write", name)
			}
		})
	}
}
