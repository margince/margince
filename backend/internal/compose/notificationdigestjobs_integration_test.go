// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The morning digest's two guarantees: one message per colleague per local day,
// and never a word about a record that colleague may no longer open.
//
// It drives the production worker rather than the claim underneath it, for the
// reason the other mail suites give: a test calling ClaimDigestRun directly
// would prove the insert is conditional and prove nothing about the lane, which
// is where a second message would actually come from.
//
// Every notice below is written by a real producer — the automation engine's
// notify seam, or the store's own Create under the system principal — and the
// ownership flip in the withholding case goes through the contacts store's own
// update. Nothing here reaches for SQL to manufacture a state the product
// cannot reach on its own.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// digestMailOrigin is the installation's own public origin, as an operator
// would configure it.
const digestMailOrigin = "https://crm.example.test"

// digestMorning is past the briefing hour in the fixture's own zone, which the
// harness seeds as UTC. The date is deliberately in the past: the window opens
// two local-day labels back, so a clock ahead of the database's would put every
// notice the test writes outside it.
var digestMorning = time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)

// digestNight is the same local day, before the morning has started.
var digestNight = time.Date(2026, 6, 4, 4, 0, 0, 0, time.UTC)

// digestEnv is one workspace with the morning pass wired as compose wires it,
// a relay that counts, and a clock the test moves.
type digestEnv struct {
	*integration.Env
	store  *notices.Store
	relay  *countingMailer
	worker *notificationDigestWorker
	now    time.Time
}

func setupDigest(t *testing.T) *digestEnv {
	t.Helper()
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	d := &digestEnv{
		Env:   e,
		store: notices.NewStore(db),
		relay: &countingMailer{},
		now:   digestMorning,
	}
	// A real role row, because the pass resolves the recipient's authority from
	// the database rather than from a principal a caller handed it — that IS the
	// withholding argument, and a fixture supplying permissions in memory would
	// prove nothing about it.
	d.grantTheReaderTheirContacts(t)
	d.worker = &notificationDigestWorker{
		pool:    e.Pool,
		notices: d.store,
		users:   identity.NewService(e.Pool),
		mail:    NotificationMailConfig{Mailer: d.relay, PublicBaseURL: digestMailOrigin},
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:     func() time.Time { return d.now },
	}
	d.batch(t, "automation")
	return d
}

// grantTheReaderTheirContacts gives the two reps the role a colleague holds for
// this lane: reading contacts, scoped to their own.
func (d *digestEnv) grantTheReaderTheirContacts(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	owner := integration.OwnerConn(t)
	var roleID ids.UUID
	if err := owner.QueryRow(ctx,
		`INSERT INTO role (key, name, permissions)
		 VALUES ('digest_reader', 'Digest Reader',
		         '{"objects":{"contact":{"read":true}},"row_scope":"own"}'::jsonb)
		 RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("seeding the digest reader role: %v", err)
	}
	for _, rep := range []ids.UUID{d.Rep1, d.Rep2} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`, roleID, rep); err != nil {
			t.Fatalf("assigning the digest reader role to %s: %v", rep, err)
		}
	}
}

// batch is the reader routing one class into the daily batch, through the
// settings surface they would use themselves.
func (d *digestEnv) batch(t *testing.T, class string) {
	t.Helper()
	if _, err := d.store.SaveNotificationPreference(d.seatCtx(d.Rep1), class, notices.DeliveryDigest); err != nil {
		t.Fatalf("routing %s into the daily batch: %v", class, err)
	}
}

// route is the reader choosing any other delivery for a class.
func (d *digestEnv) route(t *testing.T, class, delivery string) {
	t.Helper()
	if _, err := d.store.SaveNotificationPreference(d.seatCtx(d.Rep1), class, delivery); err != nil {
		t.Fatalf("routing %s to %s: %v", class, delivery, err)
	}
}

func (d *digestEnv) seatCtx(user ids.UUID) context.Context {
	return d.As(user, nil, integration.RepPerms)
}

// engineCtx is who really raises a notice: a system flow inside a correlation
// scope, with no human in the call at all.
func (d *digestEnv) engineCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), d.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:automation",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// raise writes one notice the way an automation's notify action does, and
// answers its id.
func (d *digestEnv) raise(t *testing.T, subject string, target datasource.EntityRef) ids.UUID {
	t.Helper()
	dedupe := "digest_suite:" + subject
	if err := (noticesNotifier{store: d.store}).Notify(
		d.engineCtx(), d.Rep1, subject, "", target, dedupe, nil); err != nil {
		t.Fatalf("raising the notice %q: %v", subject, err)
	}
	return d.noticeKeyed(t, dedupe)
}

// raiseOfKind writes a notice of another class through the store's own writer,
// which is what the lead-SLA escalation and the approval fan-out reach for.
func (d *digestEnv) raiseOfKind(t *testing.T, kind, subject string) {
	t.Helper()
	if _, err := d.store.Create(d.engineCtx(), notices.NewNotice{
		Recipient: ids.From[ids.UserKind](d.Rep1),
		Kind:      kind,
		Subject:   subject,
		DedupeKey: "digest_suite:" + subject,
	}); err != nil {
		t.Fatalf("raising the %s notice %q: %v", kind, subject, err)
	}
}

func (d *digestEnv) noticeKeyed(t *testing.T, dedupe string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM notice WHERE recipient_user_id = $1 AND dedupe_key = $2`,
		d.Rep1, dedupe).Scan(&id); err != nil {
		t.Fatalf("reading back the notice keyed %q: %v", dedupe, err)
	}
	return id
}

// capturedContact seeds a contact and leaves it owner-private, the state a
// connector leaves an unpromoted one in — which is what makes who owns it
// decide who may read it.
func (d *digestEnv) capturedContact(t *testing.T, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	id := d.SeedContact(t, name, &owner)
	private := "owner"
	held := ids.From[ids.UserKind](owner)
	if _, err := d.Contacts.UpdateContact(d.Admin(), integration.ContactIDOf(id),
		contacts.UpdateContactInput{OwnerID: &held, Visibility: &private}); err != nil {
		t.Fatalf("withdrawing %s to its owner: %v", name, err)
	}
	return id
}

// reassign hands a contact to another colleague through the store's own update,
// acting as the OWNER — which is who performs a handover, and the only seat an
// owner-private row answers to.
func (d *digestEnv) reassign(t *testing.T, contact, to ids.UUID) {
	t.Helper()
	held := ids.From[ids.UserKind](to)
	if _, err := d.Contacts.UpdateContact(
		d.As(d.Rep1, []ids.UUID{d.Team1}, integration.RepPerms),
		integration.ContactIDOf(contact),
		contacts.UpdateContactInput{OwnerID: &held}); err != nil {
		t.Fatalf("handing the contact over: %v", err)
	}
}

func contactRef(id ids.UUID) datasource.EntityRef {
	return datasource.EntityRef{Type: datasource.EntityContact, ID: id}
}

// run drives the worker's per-workspace turn, which is what River's row walks
// rather than what it carries: the pass takes no workspace in its args, so Work
// would enumerate the fleet and lose the one this suite is about.
func (d *digestEnv) run(t *testing.T) {
	t.Helper()
	if err := d.worker.digestOneWorkspace(context.Background(), d.WS); err != nil {
		t.Fatalf("the morning digest pass failed: %v", err)
	}
}

// claimState answers whether the day is claimed for this seat and what cause, if
// any, the row records.
func (d *digestEnv) claimState(t *testing.T, day time.Time) (bool, *string) {
	t.Helper()
	var cause *string
	err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT mail_error FROM notification_digest_run WHERE user_id = $1 AND digest_date = $2`,
		d.Rep1, day.Format(time.DateOnly)).Scan(&cause)
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return false, nil
	}
	if err != nil {
		t.Fatalf("reading the day's claim: %v", err)
	}
	return true, cause
}

// seatAddress is the reader's own address, read rather than restated: the
// harness mints it, and a literal here would pass while the mail went somewhere
// else.
func (d *digestEnv) seatAddress(t *testing.T) string {
	t.Helper()
	var address string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT email FROM app_user WHERE id = $1`, d.Rep1).Scan(&address); err != nil {
		t.Fatalf("reading the seat's address: %v", err)
	}
	return address
}

// THE CADENCE GUARANTEE. The pass ticks hourly so that it crosses the local
// morning in every zone; the claim is the only thing standing between that and
// twenty-four messages a day. The pre-dawn tick is here too, and it is the half
// that is easy to get backwards: it must leave the attempt UNSPENT, or the tick
// at five would find the day already taken and nothing would ever be sent.
func TestOneLocalMorningProducesExactlyOneDigest(t *testing.T) {
	d := setupDigest(t)
	d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})
	d.raise(t, "A renewal you own is due next week", datasource.EntityRef{})

	d.now = digestNight
	d.run(t)
	if got := d.relay.count(); got != 0 {
		t.Fatalf("a tick before the morning sent %d message(s): %v", got, d.relay.sends)
	}
	if claimed, _ := d.claimState(t, digestNight); claimed {
		t.Fatal("a tick before the morning spent the day's one attempt, so the morning tick could never send")
	}

	d.now = digestMorning
	d.run(t)
	if got := d.relay.count(); got != 1 {
		t.Fatalf("the morning sent %d message(s), want one: %v", got, d.relay.sends)
	}
	if want := d.seatAddress(t); d.relay.sends[0] != want {
		t.Errorf("the digest went to %q, not to the seat it is addressed to (%q)", d.relay.sends[0], want)
	}
	if subject := d.relay.subjects[0]; !strings.Contains(subject, "(2)") {
		t.Errorf("the subject %q does not say how much is waiting", subject)
	}
	body := d.relay.bodies[0]
	for _, line := range []string{"A proposal you own was accepted", "A renewal you own is due next week"} {
		if !strings.Contains(body, line) {
			t.Errorf("the body does not carry %q:\n%s", line, body)
		}
	}
	if want := digestMailOrigin + "/#/worklist"; !strings.Contains(body, want) {
		t.Errorf("the body does not link to the worklist (%q):\n%s", want, body)
	}
	if claimed, cause := d.claimState(t, digestMorning); !claimed || cause != nil {
		t.Errorf("a sent morning left claimed=%v cause=%v, want a clean claim", claimed, cause)
	}

	// The hourly tick comes round again inside the same local day and must find
	// the morning already spent.
	d.now = digestMorning.Add(3 * time.Hour)
	d.run(t)
	if got := d.relay.count(); got != 1 {
		t.Fatalf("a later tick on the same day sent again: %d message(s) in total", got)
	}
}

// THE WITHHOLDING GUARANTEE, and the reason this lane was written carefully.
//
// A notice's subject names the record it is about, and the notice keeps the
// reference it was written with. Ownership moves. So the message is rendered
// against the reference RE-SCOPED under the reader's own authority at send time,
// never against what was true when the line was recorded — and a record that no
// longer answers to them is counted rather than named.
//
// The flip happens between the two writes and the tick, through the contacts
// store's own update, so the notice row is exactly what the product left there.
func TestTheDigestNeverNamesARecordItsReaderMayNoLongerOpen(t *testing.T) {
	d := setupDigest(t)
	const stillMine = "Aurelia Vance"
	const handedOver = "Thorne Baxter"

	kept := d.capturedContact(t, stillMine, d.Rep1)
	moved := d.capturedContact(t, handedOver, d.Rep1)
	d.raise(t, stillMine+" replied to your note", contactRef(kept))
	d.raise(t, handedOver+" replied to your note", contactRef(moved))
	// A notice about no record at all: there is nothing to re-scope, so its own
	// sentence to this one reader is quotable.
	d.raise(t, "Your captured mail is waiting on the classifier", datasource.EntityRef{})

	// THE FLIP. The contact stops being this reader's, which is the whole of
	// what stops them reading it — it is owner-private, the state a connector
	// leaves an unpromoted contact in.
	d.reassign(t, moved, d.Rep2)

	d.run(t)

	if got := d.relay.count(); got != 1 {
		t.Fatalf("the morning sent %d message(s), want one", got)
	}
	body := d.relay.bodies[0]
	if strings.Contains(body, handedOver) {
		t.Errorf("the digest named a record its reader may no longer open:\n%s", body)
	}
	if subject := d.relay.subjects[0]; strings.Contains(subject, handedOver) {
		t.Errorf("the subject named a record its reader may no longer open: %q", subject)
	}
	if !strings.Contains(body, stillMine) {
		t.Errorf("the digest withheld a record its reader still owns:\n%s", body)
	}
	if !strings.Contains(body, "Your captured mail is waiting on the classifier") {
		t.Errorf("the digest withheld a notice that names no record at all:\n%s", body)
	}
	// The withheld line is COUNTED. A reader is told how much is waiting; they
	// are not told which of it was kept from them.
	if !strings.Contains(body, "1 more") {
		t.Errorf("the withheld notice was dropped rather than counted:\n%s", body)
	}
	if subject := d.relay.subjects[0]; !strings.Contains(subject, "(3)") {
		t.Errorf("the subject %q does not count the notice it could not name", subject)
	}
}

// A notice the reader already answered on screen is not then mailed about. The
// window asks for unread rows, so settling one before the tick takes it out of
// the morning entirely — including out of the count.
func TestANoticeReadOnScreenIsNotInThatMorningsDigest(t *testing.T) {
	d := setupDigest(t)
	answered := d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})
	d.raise(t, "A renewal you own is due next week", datasource.EntityRef{})

	if err := d.store.MarkRead(d.seatCtx(d.Rep1), answered); err != nil {
		t.Fatalf("settling the notice on screen: %v", err)
	}

	d.run(t)

	if got := d.relay.count(); got != 1 {
		t.Fatalf("the morning sent %d message(s), want one", got)
	}
	body := d.relay.bodies[0]
	if strings.Contains(body, "A proposal you own was accepted") {
		t.Errorf("the digest repeated a notice its reader had already answered:\n%s", body)
	}
	if subject := d.relay.subjects[0]; !strings.Contains(subject, "(1)") {
		t.Errorf("the subject %q counts a notice that was already answered", subject)
	}
}

// Only the classes the reader put in the batch are in it. The others reach them
// the way they asked — on screen, or as their own message — and a digest that
// swept them up would deliver twice what one of them chose once.
func TestOnlyTheClassesTheReaderBatchedAreInTheDigest(t *testing.T) {
	d := setupDigest(t)
	d.route(t, "lead_sla", notices.DeliveryEmail)
	d.route(t, "capture", notices.DeliveryInApp)
	d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})
	d.raiseOfKind(t, "lead_sla", "A lead has been waiting two days")
	d.raiseOfKind(t, "capture_backlog_stalled", "Your captured mail is waiting on the classifier")
	// Never chosen, so it follows the installation's own default, which is mail.
	d.raiseOfKind(t, notices.KindApprovalPending, "A close date correction is waiting on you")

	d.run(t)

	if got := d.relay.count(); got != 1 {
		t.Fatalf("the morning sent %d message(s), want one", got)
	}
	body := d.relay.bodies[0]
	for _, elsewhere := range []string{
		"A lead has been waiting two days",
		"Your captured mail is waiting on the classifier",
		"A close date correction is waiting on you",
	} {
		if strings.Contains(body, elsewhere) {
			t.Errorf("the digest carried %q, which its reader asked to receive another way:\n%s", elsewhere, body)
		}
	}
	if subject := d.relay.subjects[0]; !strings.Contains(subject, "(1)") {
		t.Errorf("the subject %q counts notices this digest does not carry", subject)
	}
}

// A relay that refuses the message spends the claim anyway, and says why.
//
// The trade the design accepts rather than a defect: SMTP reports no receipt, so
// a retry could not tell a refused message from a delivered one and would risk
// sending a colleague two mornings. What must not happen is the failure
// vanishing — the cause is written onto the claimed row, so a missing digest is
// answerable.
func TestARefusedDigestSpendsTheMorningAndRecordsWhy(t *testing.T) {
	d := setupDigest(t)
	d.relay.fail = errors.New("relay refused the recipient")
	d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})

	d.run(t)
	d.run(t)

	if got := d.relay.count(); got != 1 {
		t.Fatalf("a refused digest was retried %d times; there is no receipt to retry against", got)
	}
	claimed, cause := d.claimState(t, digestMorning)
	if !claimed {
		t.Fatal("a refused attempt left no claim, so the next tick would send again")
	}
	if cause == nil || !strings.Contains(*cause, "refused") {
		t.Errorf("the failure was not recorded on the row: %v", cause)
	}
}

// A colleague who put nothing in the batch is never claimed and never mailed.
// The claim is the day's one attempt, and spending it on somebody who asked for
// nothing would be spending it where it can never be used.
func TestASeatWithNothingBatchedIsNeverClaimed(t *testing.T) {
	d := setupDigest(t)
	d.route(t, "automation", notices.DeliveryInApp)
	d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})

	d.run(t)

	if got := d.relay.count(); got != 0 {
		t.Fatalf("a seat who reads their queue on screen was mailed: %v", d.relay.sends)
	}
	if claimed, _ := d.claimState(t, digestMorning); claimed {
		t.Error("a seat who asked for no batch had their day's attempt spent")
	}
}

// The roster decides who has a morning at all, and it is identity's own
// live-member rule plus the two seat questions the overnight brief asks.
//
// Both halves of the live-member pair matter and neither implies the other:
// deactivating a seat leaves archived_at NULL. A seat the roster does not name
// is skipped WITHOUT a claim — spending the day's one attempt on somebody who
// can never receive it would spend it where it can never be used, so a seat
// reinstated the same morning still gets their digest.
func TestOnlyAColleagueWithAMorningIsSentADigest(t *testing.T) {
	for _, seat := range []struct{ name, mutation string }{
		{"a deactivated colleague", `UPDATE app_user SET status = 'deactivated' WHERE id = $1`},
		{"an archived colleague", `UPDATE app_user SET archived_at = now() WHERE id = $1`},
		{"a read seat", `UPDATE app_user SET seat_type = 'read' WHERE id = $1`},
		{"an agent seat", `UPDATE app_user SET is_agent = true WHERE id = $1`},
	} {
		t.Run(seat.name, func(t *testing.T) {
			d := setupDigest(t)
			d.raise(t, "A proposal you own was accepted", datasource.EntityRef{})
			d.WsExec(t, seat.mutation, d.Rep1)

			d.run(t)

			if got := d.relay.count(); got != 0 {
				t.Fatalf("%s was sent a morning digest: %v", seat.name, d.relay.sends)
			}
			if claimed, _ := d.claimState(t, digestMorning); claimed {
				t.Errorf("%s had the day's one attempt spent on them", seat.name)
			}
		})
	}
}

// Nothing a notice carries can write a line of its own in the message.
//
// mailer.Send guards the headers; the body is the sender's to keep honest, and a
// notice's subject is written by producers that accept a newline — the store
// bounds its length and nothing strips its structure.
func TestTheDigestCarriesNoLineANoticeForged(t *testing.T) {
	d := setupDigest(t)
	d.raise(t, "Quota reached\nFrom: finance@margince.test", datasource.EntityRef{})

	d.run(t)

	if got := d.relay.count(); got != 1 {
		t.Fatalf("the morning sent %d message(s), want one", got)
	}
	if subject := d.relay.subjects[0]; strings.ContainsAny(subject, "\r\n") {
		t.Errorf("the subject carries a line break: %q", subject)
	}
	if body := d.relay.bodies[0]; strings.Contains(body, "\nFrom: finance@margince.test") {
		t.Errorf("a notice wrote a line of its own into the message:\n%s", body)
	}
}
