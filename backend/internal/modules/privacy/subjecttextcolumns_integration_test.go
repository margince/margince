// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The text columns Art. 17 erasure was leaving standing, against real rows.
//
// The column census in backend/gates reads SET clauses and can prove that each
// of these names appears in one. What it cannot see is whether the statement
// carrying the name ever reaches the row — a WHERE that does not match, or an
// arm that runs on a different path, reads exactly like one that works. So the
// erasure runs here and the rows are asked afterwards.
//
// All six were on the census's baseline before this test existed, which is to
// say they were recorded as NOT cleared: the subject's own profile address, the
// free text a rep wrote about why the lead was dropped or its score overridden,
// the pointer to their photograph and where it came from, and the provider's
// bounce text — which routinely quotes the recipient address in full, on the one
// table whose whole point is that the address copy is scrubbed.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The two arms that anonymize a subject's own rows. Both are asked, because
// they are two writers of one invariant: the Art. 17 request and the retention
// sweep clear the same columns on the same tables, and a column added to one
// alone leaves the other keeping what the first was built to remove.
var subjectRowWriters = map[string]func(context.Context, pgx.Tx, ids.PersonID, []string) error{
	"an Art. 17 request": func(ctx context.Context, tx pgx.Tx, person ids.PersonID, emails []string) error {
		// No channel accounts: this suite drives the subject's own TEXT
		// columns, and the account list reaches only the participant scrub —
		// a graph structure, covered by its own erasure test.
		_, err := anonymizeSubjectRows(ctx, tx, person, emails, nil)
		return err
	},
	"the retention sweep": func(ctx context.Context, tx pgx.Tx, person ids.PersonID, emails []string) error {
		if err := anonymizePersonRecord(ctx, tx, person.UUID); err != nil {
			return err
		}
		_, err := anonymizeLeadTwins(ctx, tx, person, emails)
		return err
	},
}

func TestErasureClearsTheSubjectTextColumnsItUsedToLeave(t *testing.T) {
	for name, anonymize := range subjectRowWriters {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := subjectColumnsTx(ctx, t)

			ws, user := ids.NewV7(), ids.NewV7()
			person := ids.New[ids.PersonKind]()
			lead := ids.NewV7()
			const subjectEmail = "bounced@anon.test"
			mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
			mustExec(ctx, t, tx,
				`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
				user, "admin-"+user.String()+"@anon.test")
			mustExec(ctx, t, tx,
				`INSERT INTO person (id, full_name, source, captured_by, photo_object_key, photo_origin)
				 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text, $3, 'human_upload')`,
				person, user, "people/"+person.String()+".jpg")
			mustExec(ctx, t, tx,
				`INSERT INTO person_email (person_id, email, source, captured_by)
				 VALUES ($1, $2, 'manual', 'user:'||$3::text)`, person, subjectEmail, user)
			// The twin the erasure reaches through promoted_person_id.
			mustExec(ctx, t, tx,
				`INSERT INTO lead (id, full_name, source, captured_by, promoted_person_id,
				                   linkedin_url, disqualify_note, score_override_reason)
				 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text, $3,
				         'https://www.linkedin.com/in/hedda-subject',
				         'said on the call she has moved to a competitor',
				         'raised by hand after she answered the second mail')`,
				lead, user, person)

			if err := anonymize(ctx, tx, person, []string{subjectEmail}); err != nil {
				t.Fatalf("anonymizing: %v", err)
			}

			assertNulled(ctx, t, tx,
				`SELECT photo_object_key, photo_origin FROM person WHERE id = $1`, person.UUID,
				"person.photo_object_key", "person.photo_origin")
			assertNulled(ctx, t, tx,
				`SELECT linkedin_url, disqualify_note, score_override_reason FROM lead WHERE id = $1`, lead,
				"lead.linkedin_url", "lead.disqualify_note", "lead.score_override_reason")
		})
	}
}

// The lead's own retention action, which reaches a lead nobody was promoted
// from — so the twin sweep above never sees it. A fourth writer of the same
// three columns, and the one a person-driven test cannot reach.
func TestTheLeadRetentionActionClearsTheSameThreeColumns(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	lead := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO lead (id, full_name, source, captured_by,
		                   linkedin_url, disqualify_note, score_override_reason)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text,
		         'https://www.linkedin.com/in/hedda-subject',
		         'said on the call she has moved to a competitor',
		         'raised by hand after she answered the second mail')`,
		lead, user)

	if err := (&RetentionService{}).anonymizeLead(ctx, tx, lead); err != nil {
		t.Fatalf("anonymizing the lead: %v", err)
	}

	assertNulled(ctx, t, tx,
		`SELECT linkedin_url, disqualify_note, score_override_reason FROM lead WHERE id = $1`, lead,
		"lead.linkedin_url", "lead.disqualify_note", "lead.score_override_reason")
}

// The two arms that scrub a delivery's addressing: the Art. 17 scrub, which
// also empties what the message SAID, and the restriction scrub, which keeps
// the words and removes who they named. Both are asked for the same reason the
// subject-row writers are — the census is satisfied by ONE statement clearing a
// column, so a second arm that stopped clearing it would fail nothing.
var deliveryScrubs = map[string]func(context.Context, pgx.Tx, []ids.UUID, PayloadPurger) error{
	"an Art. 17 request": func(ctx context.Context, tx pgx.Tx, activities []ids.UUID, payloads PayloadPurger) error {
		return redactDeliveries(ctx, tx, activities, "erased", payloads)
	},
	"a restriction": redactDeliveryAddressing,
}

func TestEveryDeliveryScrubClearsTheBounceTextThatQuotesTheAddress(t *testing.T) {
	for name, scrub := range deliveryScrubs {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := subjectColumnsTx(ctx, t)

			ws, user := ids.NewV7(), ids.NewV7()
			activity := ids.NewV7()
			mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
			mustExec(ctx, t, tx,
				`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
				user, "admin-"+user.String()+"@anon.test")
			mustExec(ctx, t, tx,
				`INSERT INTO activity (id, kind, occurred_at, source, captured_by)
				 VALUES ($1, 'email', now(), 'manual', 'user:'||$2::text)`, activity, user)
			// A real bounce quotes the address it could not reach — which is the
			// copy bounce_recipient beside it is scrubbed for. The rest of the row
			// is the mail shape `comms_outbound_shape` insists on.
			delivery := ids.NewV7()
			mustExec(ctx, t, tx,
				`INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id,
				                            recipients, cc, subject, body, references_chain,
				                            consent_purpose, status, sent_at,
				                            bounced_at, bounce_kind, bounce_recipient, bounce_reason)
				 VALUES ($1, $2, $3, 'gmail', $4,
				         jsonb_build_array($5::text), '[]'::jsonb, 'A first note', 'the message that bounced',
				         '[]'::jsonb, 'transactional', 'sent', now(),
				         now(), 'hard', $5,
				         '550 5.1.1 <hedda@anon.test>: Recipient address rejected: User unknown')`,
				delivery, activity, user, delivery.String()+"@margince.test", "hedda@anon.test")

			if err := scrub(ctx, tx, []ids.UUID{activity}, noPayloads{t: t}); err != nil {
				t.Fatalf("scrubbing the delivery: %v", err)
			}

			assertNulled(ctx, t, tx,
				`SELECT bounce_reason FROM comms_outbound WHERE activity_id = $1`, activity,
				"comms_outbound.bounce_reason")
		})
	}
}

// noPayloads stands in for the vault purger. The fixture row carries no
// payload reference, so a call here would mean the scrub found link material
// this test never seeded — which is a fact worth failing on rather than
// absorbing.
type noPayloads struct{ t *testing.T }

func (p noPayloads) Delete(_ context.Context, ref string) error {
	p.t.Errorf("the vault purger was asked to delete %q, and this fixture seeded no payload", ref)
	return nil
}

// subjectColumnsTx opens a transaction that is rolled back when the test ends,
// so the lane database is left as it was found.
func subjectColumnsTx(ctx context.Context, t *testing.T) pgx.Tx {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	})
	return tx
}

// assertNulled reads one row and reports every selected column still holding a
// value, naming it as the caller spells it.
func assertNulled(ctx context.Context, t *testing.T, tx pgx.Tx, sql string, row ids.UUID, columns ...string) {
	t.Helper()
	held := make([]*string, len(columns))
	into := make([]any, len(columns))
	for i := range held {
		into[i] = &held[i]
	}
	if err := tx.QueryRow(ctx, sql, row).Scan(into...); err != nil {
		t.Fatalf("reading %v: %v", columns, err)
	}
	for i, value := range held {
		if value != nil {
			t.Errorf("%s still holds %q after the erasure — it was on the census baseline as a "+
				"column the redaction did not clear, and this is what says it now does",
				columns[i], *value)
		}
	}
}
