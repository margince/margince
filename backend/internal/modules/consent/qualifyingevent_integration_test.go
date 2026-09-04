// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Writing down the exchange that happened away from every system, and watching
// the verdict change because of it.
//
// The verdict is the whole point, which is why this is an integration test: a
// unit test can show that a row was written, and only the real query can show
// that `business_correspondence` reads it and stops saying nobody has decided.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type qualifyingEnv struct {
	store *Store
	// owner seeds timeline rows the consent store does not own. Activities
	// belong to another module, so there is no production writer this package
	// can reach — the shape is copied from what capture writes, and the
	// authorship test the derivation applies is what makes the copy honest.
	owner          *pgx.Conn
	ctx            context.Context
	ws, user       ids.UUID
	person         ids.PersonID
	correspondence PurposeRow
}

func setupQualifying(t *testing.T) *qualifyingEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
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
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &qualifyingEnv{owner: owner, ws: ids.NewV7(), user: ids.NewV7(), person: ids.New[ids.PersonKind]()}
	purposeID := ids.New[ids.PurposeKind]()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.user, "rep-"+e.user.String()+"@qe.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, 'business_correspondence', 'Business correspondence', 'business_correspondence', false)`,
		purposeID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Dana Metatrade', 'manual', 'human:x')`, e.person); err != nil {
		t.Fatal(err)
	}
	e.correspondence = PurposeRow{
		ID: purposeID.String(), Key: "business_correspondence", Label: "Business correspondence",
		Class: ClassBusinessCorrespondence,
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))

	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	e.ctx = principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return e
}

// verdict reads what business correspondence to this person would answer now.
func (e *qualifyingEnv) verdict(t *testing.T) Verdict {
	t.Helper()
	return e.verdictSince(t, time.Now().Add(-defaultReplyWindow))
}

// verdictSince reads the verdict against an explicit window start, which is
// what lets a test move the window rather than the clock. The row keeps its
// real timestamp and only the rule's reach changes, so a failure is about the
// bound under test and not about a fixture that time-travelled.
func (e *qualifyingEnv) verdictSince(t *testing.T, since time.Time) Verdict {
	t.Helper()
	var out Verdict
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = VerdictForPerson(e.ctx, tx, e.person.String(), e.correspondence, since)
		return err
	}); err != nil {
		t.Fatalf("reading the verdict: %v", err)
	}
	return out
}

// inbound files a message the PERSON wrote, the way capture files one: an
// activity, a link putting it on their record, and a participant row naming
// them as the author. All three, because the derivation requires all three —
// seeding only the link would test a shape the product never produces and pass
// against a reader that had stopped checking authorship.
func (e *qualifyingEnv) inbound(t *testing.T, at time.Time) {
	t.Helper()
	activityID := ids.NewV7()
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'A question about your pricing', $2, 'capture', 'human:x')`,
		activityID, at); err != nil {
		t.Fatalf("seeding the inbound activity: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO activity_link (activity_id, entity_type, person_id) VALUES ($1, 'person', $2)`,
		activityID, e.person); err != nil {
		t.Fatalf("filing the inbound activity: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO activity_participant (activity_id, role, person_id, address)
		VALUES ($1, 'from', $2, 'dana@metatrade.test')`,
		activityID, e.person); err != nil {
		t.Fatalf("naming the author of the inbound activity: %v", err)
	}
}

func TestAnInPersonExchangeMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	// Before: nobody has written, no deal connects them, and the honest answer
	// is that nobody has decided.
	if before := e.verdict(t); before.State != VerdictUnknown {
		t.Fatalf("the starting verdict = %q, want unknown", before.State)
	}

	met := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	recorded, err := e.store.RecordQualifyingEvent(e.ctx, e.person, RecordQualifyingEventInput{
		Kind:       "in_person",
		Note:       "Handed me their card at the Frankfurt trade fair, stand B12.",
		OccurredAt: met,
	})
	if err != nil {
		t.Fatalf("recording the exchange: %v", err)
	}
	if recorded.Note == "" {
		t.Error("the recorded event carries no note — the note IS its evidence")
	}

	// After: the verdict reads the row and allows the send. This is the whole
	// reason the endpoint exists — an ask-email cannot be offered on a verdict
	// the transmit gate would refuse.
	after := e.verdict(t)
	if after.State != VerdictAllowed {
		t.Fatalf("the verdict after the exchange = %q (%s), want allowed", after.State, after.Reason)
	}
	if after.Qualifying == nil || after.Qualifying.Kind != "in_person" {
		t.Errorf("the verdict cites %+v, want the in-person exchange", after.Qualifying)
	}
}

func TestRecordingAnExchangeRefusesWhatItCannotStandBehind(t *testing.T) {
	e := setupQualifying(t)
	when := time.Now().Add(-time.Hour)

	cases := []struct {
		name string
		in   RecordQualifyingEventInput
	}{
		// The three derived kinds are read off records the product already
		// holds. A hand-written one would be a second, unbacked answer.
		{"a derived kind", RecordQualifyingEventInput{Kind: "inbound_message", Note: "they wrote", OccurredAt: when}},
		// An in-person exchange has no message to cite and no deal to point at.
		{"no note", RecordQualifyingEventInput{Kind: "in_person", OccurredAt: when}},
		{"a blank note", RecordQualifyingEventInput{Kind: "in_person", Note: "   ", OccurredAt: when}},
		{"no date", RecordQualifyingEventInput{Kind: "in_person", Note: "met them"}},
		// A date in the future is not a memory. It also wins the verdict's
		// `ORDER BY occurred_at DESC`, so accepting one would let a fabricated
		// row authorize sending now AND shadow the real evidence beneath it.
		{"a date in the future", RecordQualifyingEventInput{
			Kind: "in_person", Note: "will meet them", OccurredAt: time.Now().Add(48 * time.Hour),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.store.RecordQualifyingEvent(e.ctx, e.person, tc.in); err == nil {
				t.Fatal("the claim was recorded, want a refusal")
			}
		})
	}
	// None of them moved the verdict.
	if after := e.verdict(t); after.State != VerdictUnknown {
		t.Errorf("the verdict = %q after five refusals, want unknown", after.State)
	}
}

// The rows ARE the legal evidence, so a retry must not stack a second one
// claiming a second meeting happened. A double click, or a client that resends
// on a timeout, is one exchange.
func TestRecordingTheSameExchangeTwiceKeepsOneRow(t *testing.T) {
	e := setupQualifying(t)
	in := RecordQualifyingEventInput{
		Kind:       "in_person",
		Note:       "Handed me their card at the Frankfurt trade fair, stand B12.",
		OccurredAt: time.Now().Add(-24 * time.Hour).Truncate(time.Second),
	}
	for i := 0; i < 3; i++ {
		if _, err := e.store.RecordQualifyingEvent(e.ctx, e.person, in); err != nil {
			t.Fatalf("recording pass %d: %v", i+1, err)
		}
	}

	var rows int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx,
			`SELECT count(*) FROM consent_qualifying_event WHERE person_id = $1`,
			e.person).Scan(&rows)
	}); err != nil {
		t.Fatalf("counting the rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("qualifying-event rows = %d, want 1 — three sends of one exchange are one exchange", rows)
	}
	// And the claim still stands.
	if after := e.verdict(t); after.State != VerdictAllowed {
		t.Errorf("the verdict = %q, want allowed", after.State)
	}
}

func TestRecordingAnExchangeNeedsAuthorityOverTheSubject(t *testing.T) {
	e := setupQualifying(t)
	readOnly := principal.WithWorkspaceID(context.Background(), e.ws)
	readOnly = principal.WithCorrelationID(readOnly, ids.NewV7())
	readOnly = principal.WithActor(readOnly, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			// Read but not update: asserting a lawful basis changes what may be
			// SENT to this person, which is the authority to write their record.
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	_, err := e.store.RecordQualifyingEvent(readOnly, e.person, RecordQualifyingEventInput{
		Kind: "in_person", Note: "met them", OccurredAt: time.Now(),
	})
	if err == nil {
		t.Fatal("a read-only seat recorded a lawful basis")
	}
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("refusal = %v, want permission denied", err)
	}
}
