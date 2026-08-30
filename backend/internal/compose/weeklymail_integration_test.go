// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The weekly mail's one guarantee: at most one SMTP attempt per rep per week.
//
// It drives the production worker's own mailWeekly rather than the claim
// underneath it. A test that called ClaimMailAttempt directly would prove the
// UPDATE is conditional and prove nothing about the lane — the lane is where a
// second send would actually come from, because it is the thing that runs
// every six hours.

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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countingMailer records what it was handed and, when told to, refuses it.
// The relay is a true boundary, which is what makes a fake the right shape
// here rather than over-mocking.
type countingMailer struct {
	mu       sync.Mutex
	sends    []string
	subjects []string
	fail     error
	// onSend runs inside the send, so a test can observe the world at the
	// moment the relay is first dialled.
	onSend func()
}

func (m *countingMailer) Send(_ context.Context, to, subject, _ string) error {
	m.mu.Lock()
	m.sends = append(m.sends, to)
	m.subjects = append(m.subjects, subject)
	hook := m.onSend
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	return m.fail
}

func (m *countingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sends)
}

// weeklyMailClock is a Monday past the review hour, so the reps in the fixture
// are due the week that just closed.
var weeklyMailClock = time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC)

// mailEnv is a workspace worker wired exactly as jobs_weekly.go wires it,
// with the relay swapped for one that counts.
type mailEnv struct {
	*integration.Env
	worker *weeklyGenerateWorkspaceWorker
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
		worker: &weeklyGenerateWorkspaceWorker{
			engine: weekly.NewEngine(e.Pool),
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

// THE GUARANTEE. The dispatcher ticks every six hours and the mail lane runs on
// every one of them, so the week's message is offered to the relay many times
// over. Exactly one of those may reach it.
//
// A weekly retrospective delivered twice is a person told their own week twice
// on the one morning the mail exists to make calm.
func TestTheWeeklyMailIsAttemptedOnce(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	now := time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC)

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
	now := time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC)

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
	e.worker.mailWeekly(other, reviewID, time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC))

	if got := e.relay.count(); got != 0 {
		t.Fatalf("another rep's week was mailed to them: %d sends to %v", got, e.relay.sends)
	}
	// And the real owner's attempt is still theirs to spend.
	e.worker.mailWeekly(e.repCtx, reviewID, time.Date(2026, 7, 6, 7, 0, 0, 0, time.UTC))
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
	now := time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC)

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
// every Monday. The attempt is still spent and the reason recorded — an honest
// record of a mail that was not sent, rather than one sent to somebody who left.
func TestADeactivatedSeatIsNotMailedTheirWeek(t *testing.T) {
	e := setupWeeklyMail(t)
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reviewID := e.seedReview(t, week)
	owner := integration.OwnerConn(t)

	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1); err != nil {
		t.Fatal(err)
	}

	e.worker.mailWeekly(e.repCtx, reviewID, time.Date(2026, 7, 6, 6, 30, 0, 0, time.UTC))

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a deactivated seat was mailed their week: %v", e.relay.sends)
	}
	var cause *string
	if err := owner.QueryRow(context.Background(),
		`SELECT mail_error FROM weekly_review WHERE id = $1`, reviewID).Scan(&cause); err != nil {
		t.Fatal(err)
	}
	if cause == nil || !strings.Contains(*cause, "no email") {
		t.Errorf("the row does not say why no mail went out: %v", cause)
	}
}
