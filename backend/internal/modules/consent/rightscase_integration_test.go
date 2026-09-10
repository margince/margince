// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What happens to a proposal a data subject sends through their confirm link.
//
// Before this, it landed in person_confirm_submission and stopped. That table
// records what was sent — it is not work anybody owns. Nothing gave the request
// a deadline, nothing put it in the queue the DPO works through, and the
// subject was handed no reference to quote when chasing it. The statutory
// answer was owed from the moment it arrived, so the arrival is what has to
// open the case.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// caseRow is the parts of a rights case these tests assert on.
type caseRow struct {
	kind      string
	status    string
	subject   string
	person    string
	receipt   string
	channel   string
	received  time.Time
	dueAt     time.Time
	fromSubID string
}

// casesFor reads every rights case opened about the fixture's subject.
func casesFor(t *testing.T, e *channelConsentEnv) []caseRow {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT kind, status, subject_ref, coalesce(person_id::text, ''),
		       coalesce(receipt_reference, ''), coalesce(channel, ''),
		       coalesce(received_at, 'epoch'::timestamptz), due_at,
		       coalesce(source_submission_id::text, '')
		  FROM data_subject_request
		 WHERE person_id = $1
		 ORDER BY kind`, e.person)
	if err != nil {
		t.Fatalf("reading the rights cases: %v", err)
	}
	defer rows.Close()
	var out []caseRow
	for rows.Next() {
		var c caseRow
		if err := rows.Scan(&c.kind, &c.status, &c.subject, &c.person, &c.receipt,
			&c.channel, &c.received, &c.dueAt, &c.fromSubID); err != nil {
			t.Fatalf("scanning a rights case: %v", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the rights cases: %v", err)
	}
	return out
}

// A CORRECTION IS AN ART. 16 REQUEST, and the case opens in the same
// transaction as the proposal it came from. A case without its submission names
// work whose evidence is missing; a submission without its case is the defect
// this slice closes.
func TestACorrectionProposalOpensARectificationCase(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	receipts, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{ConfirmFieldFullName: "Corrected Name"},
	})
	if err != nil {
		t.Fatalf("submit a correction: %v", err)
	}

	cases := casesFor(t, e)
	if len(cases) != 1 {
		t.Fatalf("%d rights case(s) after a correction proposal, want 1 — the subject asked for a "+
			"change under Art. 16 and nobody owes them an answer", len(cases))
	}
	if cases[0].kind != "rectify" {
		t.Errorf("the case opened as %q, want rectify", cases[0].kind)
	}
	if cases[0].status != "open" {
		t.Errorf("the case opened as %q, want open — nobody has looked at it yet", cases[0].status)
	}
	if cases[0].fromSubID == "" {
		t.Error("the case names no submission, so a reader asking what was proposed has nowhere to look")
	}
	if cases[0].channel != channelConfirmLink {
		t.Errorf("the case arrived through %q, want %q — the identity checks that follow "+
			"differ by channel", cases[0].channel, channelConfirmLink)
	}
	// FulfilErasure parses subject_ref as a person id, so the two must agree or
	// fulfilment breaks on exactly the cases this slice opens.
	if cases[0].subject != cases[0].person {
		t.Errorf("subject_ref is %q while person_id is %q — the erasure fulfilment path parses "+
			"subject_ref and would not find this person", cases[0].subject, cases[0].person)
	}
	if len(receipts) != 1 || receipts[0].Reference == "" {
		t.Fatalf("%d receipt(s) returned, want 1 with a reference — the subject has nothing to "+
			"quote when asking after their request", len(receipts))
	}
	if receipts[0].Reference != cases[0].receipt {
		t.Errorf("the receipt says %q and the case stores %q — the subject would quote a "+
			"reference nobody can find", receipts[0].Reference, cases[0].receipt)
	}
	if receipts[0].Field != ConfirmFieldFullName {
		t.Errorf("the receipt names field %q, want %q — a subject correcting two fields cannot "+
			"otherwise tell which answer is about which", receipts[0].Field, ConfirmFieldFullName)
	}
}

// AN ERASURE REQUEST IS ART. 17, and it opens its own case beside any
// correction in the same submit.
func TestAnErasureRequestOpensAnErasureCaseWithAReceipt(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	receipts, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		RequestErasure: true,
	})
	if err != nil {
		t.Fatalf("submit a removal request: %v", err)
	}

	cases := casesFor(t, e)
	if len(cases) != 1 || cases[0].kind != dsrKindErasure {
		t.Fatalf("%d case(s) after a removal request, want 1 erasure — Art. 17 runs a clock and "+
			"nothing started it", len(cases))
	}
	if len(receipts) != 1 || receipts[0].Kind != dsrKindErasure {
		t.Fatalf("%d receipt(s), want 1 for the erasure", len(receipts))
	}
	if receipts[0].Field != "" {
		t.Errorf("the erasure receipt names field %q — an erasure proposes no field",
			receipts[0].Field)
	}
}

// THE DEADLINE IS ONE CALENDAR MONTH FROM RECEIPT, not thirty days and not one
// month from whenever the row happened to be written.
func TestTheDeadlineIsOneCalendarMonthFromReceipt(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	if _, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		RequestErasure: true,
	}); err != nil {
		t.Fatalf("submit a removal request: %v", err)
	}

	cases := casesFor(t, e)
	if len(cases) != 1 {
		t.Fatalf("%d case(s), want 1", len(cases))
	}
	// Computed HERE rather than through oneCalendarMonthAfter, which is the
	// function under test: asserting a value against the code that produced it
	// passes whatever that code does. Substituting thirty days for one calendar
	// month left the earlier spelling of this test green.
	received := cases[0].received
	want := time.Date(received.Year(), received.Month()+1, received.Day(),
		received.Hour(), received.Minute(), received.Second(), received.Nanosecond(),
		received.Location())
	if diff := cases[0].dueAt.Sub(want); diff > time.Second || diff < -time.Second {
		t.Errorf("the case is due %s for a request received %s, want %s — the subject is owed an "+
			"answer within one calendar month of asking, which is not thirty days",
			cases[0].dueAt, received, want)
	}
}

// ONE CALENDAR MONTH IS NOT THIRTY DAYS, and the months where they differ are
// most of them. This is a unit test rather than an integration one because the
// receipt time it needs is a February the fixture cannot arrange.
func TestOneCalendarMonthFollowsTheShortMonths(t *testing.T) {
	for _, c := range []struct {
		name     string
		received time.Time
		want     time.Time
	}{
		{
			// The case the statute is read on: there is no 31 February, so the
			// deadline lands on the last day of the month.
			name:     "the last day of a long month into a short one",
			received: time.Date(2026, time.January, 31, 9, 0, 0, 0, time.UTC),
			want:     time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC),
		},
		{
			name:     "an ordinary day",
			received: time.Date(2026, time.March, 10, 14, 30, 0, 0, time.UTC),
			want:     time.Date(2026, time.April, 10, 14, 30, 0, 0, time.UTC),
		},
		{
			name:     "across a year boundary",
			received: time.Date(2026, time.December, 15, 8, 0, 0, 0, time.UTC),
			want:     time.Date(2027, time.January, 15, 8, 0, 0, 0, time.UTC),
		},
		{
			// A CLOCK CHANGE IS NOT A MONTH OVERFLOW. Santiago moves its clock
			// back between these two dates, so AddDate lands at 23:30 on the
			// 5th rather than 00:30 on the 6th. A clamp reading the DAY NUMBER
			// called that an overflow and cut the deadline back to 31 August,
			// taking five days off the month the subject is owed.
			name:     "across a daylight-saving change",
			received: time.Date(2026, time.August, 6, 0, 30, 0, 0, santiago(t)),
			want:     time.Date(2026, time.September, 5, 23, 30, 0, 0, santiago(t)),
		},
		{
			// The same zone with no clock change between the two dates, so the
			// wall time is preserved. Beside the row above it, this is what
			// says the difference is the DST shift and not the zone.
			name:     "in the same zone with no clock change",
			received: time.Date(2026, time.September, 5, 0, 30, 0, 0, santiago(t)),
			want:     time.Date(2026, time.October, 5, 0, 30, 0, 0, santiago(t)),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := oneCalendarMonthAfter(c.received); !got.Equal(c.want) {
				t.Errorf("a request received %s is due %s, want %s", c.received, got, c.want)
			}
		})
	}
}

// A REPLAYED SUBMIT OPENS NO SECOND CASE. A double press, a prefetching mail
// client and a retry after a timeout are all ordinary here, and two cases for
// one request would queue the same work twice and hand the subject two
// references for one answer.
//
// The spent link refuses a second submit outright, so this drives the case
// writer itself with the submission the first submit filed — the path a retry
// inside the transaction would take, and the one the unique index has to hold.
func TestAReplayedProposalOpensNoSecondCase(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	first, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		RequestErasure: true,
	})
	if err != nil {
		t.Fatalf("submit a removal request: %v", err)
	}

	var submissionID ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT source_submission_id FROM data_subject_request WHERE person_id = $1`,
		e.person).Scan(&submissionID); err != nil {
		t.Fatalf("reading the submission the case came from: %v", err)
	}

	tx, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatalf("opening a transaction: %v", err)
	}
	// Nothing here commits, so this rollback IS the cleanup and a failure is
	// this test's own connection in trouble rather than a designed no-op.
	// ErrTxClosed is the one exception, which is what a clean run leaves.
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the replay transaction: %v", err)
		}
	})

	replayed, err := openRightsCaseTx(context.Background(), tx, e.person, submissionID,
		submissionErasure, time.Now())
	if err != nil {
		t.Fatalf("replaying the case: %v", err)
	}
	if replayed != first[0].Reference {
		t.Errorf("the replay answered reference %q where the first submit earned %q — one request "+
			"now has two cases and the subject two references for one answer",
			replayed, first[0].Reference)
	}

	var opened int
	if err := tx.QueryRow(context.Background(),
		`SELECT count(*) FROM data_subject_request WHERE person_id = $1`, e.person).Scan(&opened); err != nil {
		t.Fatalf("counting the cases: %v", err)
	}
	if opened != 1 {
		t.Errorf("%d case(s) after the replay, want 1", opened)
	}
}

// A MARKETING ANSWER ALONE OPENS NO CASE. Saying yes or no to a newsletter is
// not a request under Art. 16 or 17, and a case opened for it would put work in
// the DPO's queue that nobody asked for.
func TestAMarketingAnswerAloneOpensNoCase(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	receipts, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		MarketingChoice:  string(StateGranted),
		MarketingWording: "News from time to time.",
	})
	if err != nil {
		t.Fatalf("submit a marketing answer: %v", err)
	}
	if len(receipts) != 0 {
		t.Errorf("%d receipt(s) for a marketing answer, want none", len(receipts))
	}
	if cases := casesFor(t, e); len(cases) != 0 {
		t.Errorf("%d rights case(s) opened by a marketing answer, want none — the DPO's queue "+
			"would fill with work nobody asked for", len(cases))
	}
}

// santiago is a zone that moves its clock inside the window these deadlines
// span. Chile rather than a European zone because its transition falls in early
// September, which puts it between a receipt and its one-month deadline for a
// whole range of ordinary August dates.
func santiago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	return loc
}

// A COLLIDING RECEIPT IS RECOGNISED AS ONE. Two cases cannot both hold a
// quotable reference, so the unique index refuses the second — and inside the
// submit transaction that refusal would roll back the proposal, the case AND
// the spent token, answering a real erasure request with a server error and no
// link left to retry on. The writer redraws instead, and this is the test of
// what it redraws FOR: every other refusal must reach the caller unretried,
// because retrying a real fault turns one honest error into three and a slower
// answer.
//
// A unit test rather than an integration one. Making a receipt collide on the
// first draw and not the second needs state that survives a savepoint rollback
// and is not visible to the transaction — which the retry undoes by design, and
// which is exactly the property under test. What can be tested honestly is the
// judgement: which refusals count as a collision.
func TestOnlyAReceiptCollisionIsWorthRedrawing(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "the receipt index refusing a duplicate reference",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "data_subject_request_receipt_reference",
			},
			want: true,
		},
		{
			// The OTHER unique index on this table, and the one the writer
			// answers by reading the standing case back rather than redrawing.
			// Redrawing here would spin three times and then report a failure
			// on what is the ordinary replay.
			name: "the submission index refusing a replayed submit",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "data_subject_request_one_case_per_submission",
			},
			want: false,
		},
		{
			name: "a check constraint refusing an unknown channel",
			err:  &pgconn.PgError{Code: "23514", ConstraintName: "data_subject_request_channel"},
			want: false,
		},
		{
			name: "a connection that went away",
			err:  errors.New("write tcp: broken pipe"),
			want: false,
		},
		{
			// Wrapped, because that is how it arrives: the insert wraps its
			// failure before the retry ever sees it, and a matcher testing the
			// concrete type rather than errors.As would call every collision a
			// permanent fault.
			name: "a collision wrapped by the writer",
			err: fmt.Errorf("consent: opening the rights case: %w", &pgconn.PgError{
				Code: "23505", ConstraintName: "data_subject_request_receipt_reference",
			}),
			want: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := isReceiptCollision(c.err); got != c.want {
				t.Errorf("isReceiptCollision(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// A FAILURE THAT IS NOT A RECEIPT COLLISION IS RETURNED AT ONCE, not redrawn.
//
// The redraw exists for exactly one fault — two cases minting the same
// reference — and spending three attempts on anything else holds the
// transaction open while reporting the same error three times. So the loop has
// to tell them apart, and nothing held that.
//
// It also exercises the savepoint's rollback on the path where it matters: the
// rollback has to TAKE, because every attempt after a rollback that did not
// would fail on the aborted transaction rather than on what actually went
// wrong. The proof is in the error that comes back — the foreign-key fault the
// first attempt hit, not the abort a second one would have reported.
func TestAFailureThatIsNotAReceiptCollisionIsNotRedrawn(t *testing.T) {
	e := setupChannelConsent(t)
	// A subject with no person row, so the insert fails on the foreign key.
	stranger := ids.From[ids.PersonKind](ids.NewV7())

	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, caseErr := openRightsCaseTx(context.Background(), tx, stranger, ids.NewV7(),
			submissionErasure, time.Now())
		return caseErr
	})
	if err == nil {
		t.Fatal("opening a rights case for a subject with no record succeeded — this test " +
			"no longer reaches the failure arm it is about")
	}
	if !storekit.IsForeignKeyViolation(err) {
		t.Errorf("error = %v, want the foreign-key fault the FIRST attempt hit; anything else "+
			"means the redraw ran and reported what the aborted transaction said instead", err)
	}
}
