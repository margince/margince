// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Production workers exercise the database claims under repeated delivery attempts.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingMailer records what it was handed and, when told to, refuses it.
// The relay is a true boundary, which is what makes a fake the right shape
// here rather than over-mocking.
type countingMailer struct {
	mu       sync.Mutex
	sends    []string
	subjects []string
	// bodies is what a suite asserting on the MESSAGE reads. The weekly's own
	// cases care only that a message went out once and named the week, but the
	// notification leg's claim is about what the body says — the notice's line,
	// the link, and that neither carries a line the notice forged.
	bodies []string
	markup []string
	fail   error
	// onSend runs inside the send, so a test can observe the world at the
	// moment the relay is first dialled.
	onSend func()
}

func (m *countingMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	m.sends = append(m.sends, to)
	m.subjects = append(m.subjects, subject)
	m.bodies = append(m.bodies, body)
	hook := m.onSend
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	return m.fail
}

func (m *countingMailer) SendAlternative(ctx context.Context, to, subject, text, markup, messageID string) error {
	m.mu.Lock()
	m.markup = append(m.markup, markup)
	m.mu.Unlock()
	return m.Send(ctx, to, subject, text)
}

func (m *countingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sends)
}

// weeklyMailClock is a Monday past the review hour, so the reps in the fixture
// are due the week that just closed.
var weeklyMailClock = time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)

// mailEnv is a workspace worker wired exactly as jobs_weekly.go wires it,
// with the relay swapped for one that counts.
type mailEnv struct {
	*integration.Env
	worker *weeklyGenerateWorker
	relay  *countingMailer
	repCtx context.Context
}

func setupWeeklyMail(t *testing.T) *mailEnv {
	t.Helper()
	e := integration.Setup(t)
	relay := &countingMailer{}
	return &mailEnv{
		Env:   e,
		relay: relay,
		worker: &weeklyGenerateWorker{
			engine: weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)),
			pool:   e.Pool,
			users:  identity.NewService(e.Pool),
			now:    func() time.Time { return weeklyMailClock },
			log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
			mail:   WeeklyMailConfig{Mailer: relay, PublicBaseURL: "https://crm.example.test"},
		},
		repCtx: e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
	}
}

// seedReview writes one rep's review the way the deterministic pass leaves it.
func (e *mailEnv) seedReview(t *testing.T, week time.Time) ids.UUID {
	t.Helper()
	return integration.SeedIDRow(t, integration.OwnerConn(t), `
		INSERT INTO weekly_review (id, user_id, local_week_start, as_of)
		VALUES ($1, $2, $3, now())`, e.Rep1, week)
}

func TestTheWeeklyMailIsAttemptedOnce(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)

	// Four ticks, the shape a week of the real schedule has.
	for range 4 {
		e.worker.mailWeekly(e.repCtx, reviewID, now)
	}

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the relay was handed the week %d times; the mail is at-most-once", got)
	}
	if !strings.Contains(e.relay.subjects[0], "2026-06-29") {
		t.Errorf("the subject does not name the week: %q", e.relay.subjects[0])
	}
}

// A relay that refuses the message spends the attempt anyway, and says why.
//
// This is the trade the design accepts rather than a defect: the transport
// reports no receipt, so a retry could not tell a refused message from a
// delivered one and would risk mailing the week twice. What must not happen is
// the failure vanishing — the cause is written onto the row so a missing
// weekly is answerable.
func TestARefusedMailSpendsTheAttemptAndRecordsWhy(t *testing.T) {
	e := setupWeeklyMail(t)
	e.relay.fail = errors.New("relay refused the recipient")
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)

	e.worker.mailWeekly(e.repCtx, reviewID, now)
	e.worker.mailWeekly(e.repCtx, reviewID, now)

	if got := e.relay.count(); got != 1 {
		t.Fatalf("a refused mail was retried %d times; there is no receipt to retry against", got)
	}

	var stamp *time.Time
	var cause *string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT mail_attempted_at, mail_error FROM weekly_review WHERE id = $1`,
		reviewID).Scan(&stamp, &cause); err != nil {
		t.Fatal(err)
	}
	if stamp == nil {
		t.Error("a refused attempt left no stamp, so the next tick would send again")
	}
	if cause == nil || !strings.Contains(*cause, "refused") {
		t.Errorf("the failure was not recorded on the row: %v", cause)
	}
}

// One rep's claim must not reach another's week. The id alone is not
// authority: a review belongs to the rep whose week it was.
func TestOneRepCannotBurnAnothersAttempt(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)

	other := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	e.worker.mailWeekly(other, reviewID, time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC))

	if got := e.relay.count(); got != 0 {
		t.Fatalf("another rep's week was mailed to them: %d sends to %v", got, e.relay.sends)
	}
	// And the real owner's attempt is still theirs to spend.
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if got := e.relay.count(); got != 1 {
		t.Errorf("the owner's attempt was consumed by somebody else: %d sends", got)
	}
}

// An installation with no relay mails nothing and spends no attempt — so
// configuring mail later still reaches that week.
func TestNoRelayLeavesTheAttemptUnspent(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)

	offline := *e.worker
	offline.mail = WeeklyMailConfig{}
	offline.mailWeekly(e.repCtx, reviewID, now)

	var stamp *time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT mail_attempted_at FROM weekly_review WHERE id = $1`, reviewID).Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	if stamp != nil {
		t.Fatal("a worker with no relay burned the week's attempt, so configuring " +
			"mail later would never reach that week")
	}

	// Configured now, the week still goes out.
	e.worker.mailWeekly(e.repCtx, reviewID, now)
	if got := e.relay.count(); got != 1 {
		t.Errorf("the week did not go out once mail was configured: %d sends", got)
	}
}

// A deactivated seat is not mailed their week.
//
// Deactivating an account sets status and leaves archived_at NULL, so a
// recipient read on archived_at alone would go on mailing a departed colleague
// every Monday. An inactive seat cannot spend its mail attempt.
func TestADeactivatedSeatLeavesItsWeekUnclaimed(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	owner := integration.OwnerConn(t)

	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1); err != nil {
		t.Fatal(err)
	}

	e.worker.mailWeekly(e.repCtx, reviewID, time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC))

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a deactivated seat was mailed their week: %v", e.relay.sends)
	}
	var attempted *time.Time
	if err := owner.QueryRow(context.Background(), `SELECT mail_attempted_at FROM weekly_review WHERE id=$1`, reviewID).Scan(&attempted); err != nil {
		t.Fatal(err)
	}
	if attempted != nil {
		t.Fatal("an inactive seat spent a mail attempt")
	}
}

// THE ORDERING, as behaviour rather than as a shape read off the source.
//
// The workspace job has ten minutes, one send may take 45 seconds, and the
// loop is serial. Interleaved, one stalled relay spends the whole budget on
// the first few reps and everyone after them loses THE REVIEW — the counts and
// the deal lines, not just the mail — and loses it for good, because the
// candidate query only ever asks about the week that just closed.
//
// So the assertion is taken at the moment the relay is FIRST dialled: by then
// every rep the pass found due must already have their week written. That is
// false the instant the send moves inside the measuring loop, and it is a
// question only a database can answer.
func TestEveryRepIsMeasuredBeforeTheFirstRelayIsDialled(t *testing.T) {
	e := setupWeeklyMail(t)
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		e.GrantRole(t, rep, "rep")
	}

	var atFirstDial int
	var dials int
	e.relay.onSend = func() {
		dials++
		if dials == 1 {
			atFirstDial = countWeeklyReviews(t)
		}
	}

	if err := e.worker.measureWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), e.WS, weeklyMailClock); err != nil {
		t.Fatalf("measuring the workspace: %v", err)
	}

	// Without this the case passes vacuously on a pass that mailed nobody,
	// which is exactly what it looked like before the harness could grant a
	// seat its role.
	if dials == 0 {
		t.Fatal("the relay was never dialled, so nothing here observed the ordering: the pass " +
			"measured nobody, or every seat was refused before it reached a review")
	}
	total := countWeeklyReviews(t)
	// The floor, and it is the whole reason GrantRole exists. With ONE rep
	// measured, atFirstDial and total are both 1 whichever arrangement the code
	// takes — the case passes over the defect it is named for. Verified by
	// running it both ways: send inlined and grants removed, it is green.
	if total < 2 {
		t.Fatalf("only %d rep was measured, and one rep cannot show an ordering: this case is "+
			"green against the interleaved arrangement too, which is the state it was written "+
			"to leave behind", total)
	}
	if atFirstDial != total {
		t.Errorf("%d of %d reviews were written when the first relay was dialled; the rest were "+
			"measured after it. A relay that stalls there costs every rep behind it their review, "+
			"not their mail", atFirstDial, total)
	}
}

// countWeeklyReviews reads how many reps have their week written.
func countWeeklyReviews(t *testing.T) int {
	t.Helper()
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM weekly_review`).Scan(&n); err != nil {
		t.Fatalf("counting weekly reviews: %v", err)
	}
	return n
}

func TestAWeeklyPreferenceReadFailureDefersWithoutSpendingTheAttempt(t *testing.T) {
	e := setupWeeklyMail(t)
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	broken, err := testdb.OwnPoolFromConfig(t.Context(), e.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	broken.Close()
	e.worker.users = identity.NewService(broken)
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if e.relay.count() != 0 {
		t.Fatal("an unreadable preference permitted delivery")
	}
	var attempted *time.Time
	if err := integration.OwnerConn(t).QueryRow(e.repCtx, `SELECT mail_attempted_at FROM weekly_review WHERE id=$1`, reviewID).Scan(&attempted); err != nil {
		t.Fatal(err)
	}
	if attempted != nil {
		t.Fatal("preference outage consumed the attempt")
	}
}

func TestWeeklyOptOutKeepsAnUnspentAttemptForAnExplicitOptIn(t *testing.T) {
	e := setupWeeklyMail(t)
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	none, email := "none", "email"
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Weekly: &none}); err != nil {
		t.Fatal(err)
	}
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	pending, err := e.worker.engine.MailPending(e.repCtx, reviewID)
	if err != nil || !pending || e.relay.count() != 0 {
		t.Fatalf("opt-out consumed or sent the review: %v %v %d", pending, err, e.relay.count())
	}
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Weekly: &email}); err != nil {
		t.Fatal(err)
	}
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if e.relay.count() != 1 {
		t.Fatal("explicit opt-in could not use the unspent weekly attempt")
	}
}
