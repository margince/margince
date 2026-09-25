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
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
			return anonymizeContactRecord(ctx, tx, s.contact.UUID)
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
	for name, act := range dedupeEvidenceActs {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := subjectColumnsTx(ctx, t)
			subjects, contactPair, leadPair := seedDedupePairs(ctx, t, tx)

			if err := act.run(ctx, tx, subjects); err != nil {
				t.Fatalf("anonymizing: %v", err)
			}

			assertDedupeEvidence(ctx, t, tx, contactPair, "the contact pair", act.contact)
			assertDedupeEvidence(ctx, t, tx, leadPair, "the lead pair", act.leads)
		})
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

// fileDedupePair inserts one candidate, answering its id.
//
// The two ends are ordered here rather than by the caller because
// dedupe_candidate_ordered refuses the other spelling, and a fixture that
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
			"signal": "collide", "score": 0.91,
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
		VALUES ($1, $2, $3, $4, 0.91, $5, 'manual', 'user:'||$6::text)`,
		id, entity, left, right, evidence, user)
	return id
}

// assertDedupeEvidence reads one candidate back and reports the gap between
// what the act had to destroy and what it did.
//
// It asks in both directions. An act that emptied a pair it has no business
// reaching would be scrubbing somebody else's open decision, which is a defect
// of its own and one a one-sided assertion cannot see.
func assertDedupeEvidence(ctx context.Context, t *testing.T, tx pgx.Tx, id ids.UUID, pair string, wantEmpty bool) {
	t.Helper()
	var evidence json.RawMessage
	if err := tx.QueryRow(ctx,
		`SELECT evidence FROM dedupe_candidate WHERE id = $1`, id).Scan(&evidence); err != nil {
		t.Fatalf("reading %s back: %v", pair, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(evidence, &rows); err != nil {
		t.Fatalf("%s holds evidence that is not the detector's array shape: %v", pair, err)
	}
	switch {
	case wantEmpty && len(rows) != 0:
		t.Errorf("%s still holds %d evidence row(s) after the act: %s — the subject's name and "+
			"address survive in the snapshot the anonymize was supposed to reach", pair, len(rows), evidence)
	case !wantEmpty && len(rows) == 0:
		t.Errorf("%s was emptied by an act that does not name its subject — the predicate reaches "+
			"pairs beyond the record being anonymized", pair)
	}
}

// An erasure step that CANNOT write says so.
//
// The three steps this change added all end in a table the cascade does not
// own, and each one is the last thing standing between a subject and a copy of
// their name. A swallowed failure here is the worst shape the erasure can take:
// the transaction commits, the certificate says destroyed, and the row is still
// readable.
//
// An aborted transaction is what that looks like from inside the cascade —
// every later statement is refused — and it is the one fault a test can stage
// without taking the table away from every other suite on the lane.
func TestAScrubThatCannotWriteReportsItRatherThanCommitting(t *testing.T) {
	ctx := context.Background()
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
		"the copies outside the subject's rows": func() error {
			return clearSubjectNameCopies(ctx, tx, subject, nil)
		},
		"the rows derived about the subject": func() error {
			return deleteDerivedContactRows(ctx, tx, subject.UUID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := scrub(); err == nil {
				t.Errorf("clearing %s reported success on a transaction that can no longer write", name)
			}
		})
	}
}
