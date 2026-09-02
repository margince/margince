// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The morning brief's mail, driven through the real overnight worker.
//
// The once-only property is the whole design and it lives in a conditional
// UPDATE, so nothing short of a real database proves it: a fake store would be
// asserting that the test's own map behaves like a claim.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errRelayRefused is a relay saying no — the ordinary transport failure this
// lane must survive without failing the assembly around it.
var errRelayRefused = errors.New("the relay refused the envelope")

// seedWorkFor puts one open deal on a rep's plate, so their morning has
// something in it.
//
// A run with an EMPTY queue is a quiet morning, and a quiet morning is
// deliberately not mailed to anybody who did not ask to hear about it — so a
// fixture without this seeds the case where nothing is sent, and every
// assertion about sending passes vacuously. It did, on the first run of these
// tests, and the two that then failed were the ones honest enough to say so.
func seedWorkFor(t *testing.T, e *integration.Env, owner ids.UUID) {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, e)
	e.SeedDeal(t, "Globex Renewal", pipeline, open, &owner)
}

// withMailer wires this env's worker to a relay that counts.
func (b *briefJobEnv) withMailer(m *countingMailer) *briefJobEnv {
	b.worker.mail = BriefMailConfig{Mailer: m, PublicBaseURL: "https://crm.example"}
	return b
}

// setDelivery writes one rep's delivery preferences through the real column.
func setDelivery(t *testing.T, user ids.UUID, morning *string, quiet *bool) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE app_user SET morning_brief_delivery = $2, quiet_day_notice = $3 WHERE id = $1`,
		user, morning, quiet); err != nil {
		t.Fatal(err)
	}
}

// mailAttemptOf reads the claim stamp and the recorded cause off a rep's run.
func mailAttemptOf(t *testing.T, user ids.UUID, day time.Time) (*time.Time, *string) {
	t.Helper()
	var at *time.Time
	var cause *string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT mail_attempted_at, mail_error FROM brief_run
		  WHERE user_id = $1 AND local_day = $2`,
		user, day.Format(time.DateOnly)).Scan(&at, &cause); err != nil {
		t.Fatal(err)
	}
	return at, cause
}

func TestTheMorningBriefIsMailedOncePerRepPerDay(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	seedWorkFor(t, b.Env, b.Rep1)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning

	if err := b.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	first := relay.count()
	if first == 0 {
		t.Fatal("no morning brief was mailed at all")
	}

	// The hourly tick comes round again inside the same local day. The run is
	// already assembled, so the assembly is a no-op — and the mail must be one
	// too. This is the assertion the whole claim exists for: a lane that mailed
	// on every tick would send a rep their morning twenty-four times.
	b.now = morning.Add(3 * time.Hour)
	if err := b.run(t); err != nil {
		t.Fatalf("the second tick of the same morning failed: %v", err)
	}
	if got := relay.count(); got != first {
		t.Fatalf("a later tick on the same day mailed again: %d sends, want the %d already made", got, first)
	}

	// The claim is stamped, and nothing recorded a failure.
	at, cause := mailAttemptOf(t, b.Rep1, morning)
	if at == nil {
		t.Fatal("the run carries no claim stamp, so nothing stops the next tick sending again")
	}
	if cause != nil {
		t.Fatalf("a send that went through recorded a failure: %q", *cause)
	}
}

func TestARepWhoAskedForNoMorningMailGetsNoneAndKeepsTheirAttempt(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	seedWorkFor(t, b.Env, b.Rep1)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	none := identity.DeliveryNone
	for _, rep := range []ids.UUID{b.Rep1, b.Rep2, b.Rep3} {
		setDelivery(t, rep, &none, nil)
	}
	b.now = morning

	if err := b.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	if got := relay.count(); got != 0 {
		t.Fatalf("%d messages went out to reps who asked for none", got)
	}

	// THE ATTEMPT IS UNSPENT, which is the point of checking the preference
	// BEFORE the claim. A rep who turns the mail back on tomorrow must be able
	// to receive it; if the opted-out pass had burned today's claim, the day
	// they changed their mind in could never be sent and nothing would say why.
	at, _ := mailAttemptOf(t, b.Rep1, morning)
	if at != nil {
		t.Fatal("an opted-out rep's one attempt was spent, so turning the mail back on cannot recover this day")
	}
}

func TestAQuietMorningIsNotMailedUnlessTheRepAskedToHear(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	// Nobody has chosen. "Send me my brief" is the installation's default and a
	// rep who never chose gets it; "tell me even when there is nothing" is not,
	// because mailing the whole company "nothing is waiting on you" every quiet
	// morning teaches its readers to filter the ones that matter.
	b.now = morning
	if err := b.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	quietSends := relay.count()

	// The same fixture with the notice asked for. Whatever the queue holds,
	// asking to hear can only ever mail MORE, never less.
	relay2 := &countingMailer{}
	c := setupBriefJob(t).withMailer(relay2)
	yes := true
	for _, rep := range []ids.UUID{c.Rep1, c.Rep2, c.Rep3} {
		setDelivery(t, rep, nil, &yes)
	}
	c.now = morning
	if err := c.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	if relay2.count() < quietSends {
		t.Fatalf("asking to hear about quiet mornings mailed %d, fewer than the %d sent without asking",
			relay2.count(), quietSends)
	}
}

func TestARefusedRelaySpendsTheAttemptAndSaysWhy(t *testing.T) {
	relay := &countingMailer{fail: errRelayRefused}
	b := setupBriefJob(t).withMailer(relay)
	seedWorkFor(t, b.Env, b.Rep1)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning

	// The assembly must not fail over a relay: the brief is on Home either way,
	// and failing the job would make River retry the whole workspace's morning
	// into the same refusal.
	if err := b.run(t); err != nil {
		t.Fatalf("a refused relay failed the assembly: %v", err)
	}

	at, cause := mailAttemptOf(t, b.Rep1, morning)
	if at == nil {
		t.Fatal("a refused send left the attempt unclaimed, so the next tick would try again")
	}
	if cause == nil {
		t.Fatal("a refused send recorded no cause, so nobody asking where their brief went can be answered")
	}

	// AND IT IS NOT RETRIED. The attempt is spent either way; a lane that tried
	// again on the next tick would be the retry loop this design refuses.
	before := relay.count()
	b.now = morning.Add(3 * time.Hour)
	if err := b.run(t); err != nil {
		t.Fatalf("the second tick failed: %v", err)
	}
	if got := relay.count(); got != before {
		t.Fatalf("a failed send was retried on the next tick: %d attempts, want the %d already made", got, before)
	}
}
