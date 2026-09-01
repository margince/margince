// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What may close an introduction, and — mostly — what may not.
//
// `replied` is the workflow's best outcome and the one status no endpoint can
// set, so this consumer is the whole of its reachability. Every clause in the
// predicate is a case somebody would otherwise close an ask with: an outbound
// mail the rep sent, a message the contact was merely cc'd on, and the years of
// backfilled correspondence a first mailbox import delivers all at once.
//
// The refusals need the admit case beside them, which TestAContactsAnswer
// provides: without it a consumer that refused everything would pass every
// negative test in this file.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/introductions"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// introPerms are the grants both parties to an ask hold.
var introPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"introduction": {Create: true, Read: true, Update: true},
		"person":       {Read: true},
		"activity":     {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeAll,
}

// introFixture is one ask walked to the handshake, and the contact it is about.
type introFixture struct {
	ask       ids.UUID
	contact   ids.UUID
	store     *introductions.Store
	advance   *compose.IntroAdvance
	handshake time.Time
}

// seedIntroduced makes an ask, has the colleague accept it, and records the
// handshake — through the real store every time, so the row the consumer reads
// is the row the product writes.
func seedIntroduced(t *testing.T, e *Env) introFixture {
	t.Helper()
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	store := introductions.NewStore(e.DB(), time.Now)
	requester := e.As(e.Rep1, nil, introPerms)
	introducer := e.As(e.Rep2, nil, introPerms)

	id, err := store.Create(requester, introductions.NewRequest{
		PersonID:       contact,
		IntroducerUser: e.Rep2,
		RouteType:      "direct",
		InternalReason: "Dana reopened the retrofit conversation.",
		DueAt:          time.Now().AddDate(0, 0, 7),
	})
	if err != nil {
		t.Fatalf("asking for the introduction: %v", err)
	}
	if err := store.Decide(introducer, id, introductions.StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("the colleague accepting: %v", err)
	}
	if err := store.Complete(introducer, id, nil, 2); err != nil {
		t.Fatalf("recording the handshake: %v", err)
	}
	// The instant the ask started waiting, read back from the row rather than
	// assumed: every message below is dated relative to it, and a test that
	// guessed this would be testing its own arithmetic.
	var handshake time.Time
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT introduced_at FROM intro_request WHERE id = $1`, id).Scan(&handshake); err != nil {
		t.Fatalf("reading the handshake instant: %v", err)
	}
	return introFixture{
		ask: id, contact: contact, store: store, handshake: handshake,
		advance: compose.NewIntroAdvance(e.Pool, store, slog.New(slog.DiscardHandler)),
	}
}

// message writes one captured activity with the direction, sender role and
// instant a case needs, then hands the consumer its envelope.
//
// Written with the owner pool rather than through the activities store because
// the cases below need combinations the capture path does not readily produce —
// an outbound message from a contact, a cc without a from. What the consumer
// reads is the row, and this writes the row.
func (fx introFixture) message(
	t *testing.T, e *Env, direction, role string, personID ids.UUID, at time.Time,
) kevents.Envelope {
	t.Helper()
	return fx.messageCapturedBy(t, e, connectorProvenance, direction, role, personID, at)
}

// connectorProvenance is what capture stamps on a row it wrote: the connector
// and the mailbox owner behind it, derived from the authenticated principal
// (capture/sinkprovenance.go). Spelled the way the real writer spells it,
// because a fixture that invented its own would prove nothing about the rows
// production actually holds.
const connectorProvenance = "connector:gmail:018f3a1b-0000-7000-8000-000000000099"

// humanProvenance is what a member logging an activity by hand gets:
// storekit.CapturedBy stamps the authenticated principal's id, and a person's
// reads `human:<id>`.
const humanProvenance = "human:018f3a1b-0000-7000-8000-000000000098"

// messageCapturedBy writes one activity with the provenance a case needs.
func (fx introFixture) messageCapturedBy(
	t *testing.T, e *Env, capturedBy, direction, role string, personID ids.UUID, at time.Time,
) kevents.Envelope {
	t.Helper()
	activity := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: retrofit', $2, $3, 'sync', $4)`,
		activity, at, direction, capturedBy)
	e.WsExec(t, `
		INSERT INTO activity_participant (activity_id, person_id, role)
		VALUES ($1, $2, $3)`, activity, personID, role)
	e.WsExec(t, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, activity, personID)
	return kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "activity.captured",
		Entity:  kevents.EntityRef{Type: "activity", ID: activity},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	}
}

// statusOf reads where the ask stands.
func (fx introFixture) statusOf(t *testing.T, e *Env) string {
	t.Helper()
	var status string
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT status FROM intro_request WHERE id = $1`, fx.ask).Scan(&status); err != nil {
		t.Fatalf("reading the ask: %v", err)
	}
	return status
}

// deliver hands the consumer one envelope, failing on anything but a clean
// return: a consumer that errored would wedge its group in production.
func (fx introFixture) deliver(t *testing.T, env kevents.Envelope) {
	t.Helper()
	if err := fx.advance.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("the consumer refused an envelope: %v", err)
	}
}

// The admit case. Without it every refusal below would pass against a consumer
// that closed nothing at all.
func TestAContactsAnswerClosesTheIntroduction(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	fx.deliver(t, fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(time.Hour)))

	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("the ask is %q after the contact answered; want replied", got)
	}
}

// A mail the rep SENT is not the contact answering.
//
// Outbound traffic to a contact you have just been introduced to is the most
// common thing in the box — the rep follows up immediately. Counting it would
// mark every introduction answered within minutes of being made, by the rep
// themselves.
func TestTheRepsOwnFollowUpDoesNotCountAsAnAnswer(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	fx.deliver(t, fx.message(t, e, "outbound", "from", fx.contact, fx.handshake.Add(time.Hour)))

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("an outbound mail moved the ask to %q; want it left at introduced", got)
	}
}

// Being cc'd is not writing back.
//
// A colleague's mail that copies the contact puts them on the participant list
// without their having written a word. Only role `from` is the contact
// speaking, and this is the clause that says so.
func TestBeingCopiedOnAMessageIsNotAnAnswer(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	fx.deliver(t, fx.message(t, e, "inbound", "cc", fx.contact, fx.handshake.Add(time.Hour)))

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("a cc moved the ask to %q; want it left at introduced", got)
	}
}

// Old correspondence is not an answer to an introduction made this morning.
//
// This is the clause a first mailbox import would otherwise break wholesale:
// capture backfills years of threads at once, every one of them inbound and
// from the contact. Without the instant comparison, that single import would
// mark every introduction ever recorded as answered.
func TestMailFromBeforeTheHandshakeDoesNotCloseIt(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	fx.deliver(t, fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(-48*time.Hour)))

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("a backfilled old mail moved the ask to %q; want introduced", got)
	}

	// And the same ask still answers to a message that genuinely follows it,
	// so the refusal above is about the instant and not about the fixture.
	fx.deliver(t, fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(time.Hour)))
	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("a later message left the ask at %q; want replied", got)
	}
}

// Somebody else's mail closes nobody's introduction.
func TestAnotherContactsMailLeavesTheAskAlone(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)
	other := e.SeedPerson(t, "Unrelated Person", &e.Rep1)

	fx.deliver(t, fx.message(t, e, "inbound", "from", other, fx.handshake.Add(time.Hour)))

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("another contact's mail moved the ask to %q; want introduced", got)
	}
}

// The bus is at-least-once, so the same envelope arrives twice as a matter of
// course. The second delivery must write nothing and error nothing — an error
// would wedge the group on a message that will never stop being a duplicate.
func TestARedeliveredMessageChangesNothing(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)
	env := fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(time.Hour))

	fx.deliver(t, env)
	fx.deliver(t, env)

	replied := e.WsCount(t,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.replied'`)
	if replied != 1 {
		t.Errorf("%d replied events after two deliveries of one message; want one", replied)
	}
}

// A member cannot forge a reply by logging one.
//
// This is the escalation the captured_by clause exists to stop, and it is worth
// stating exactly, because the consumer runs as PrincipalSystem and therefore
// bypasses both object RBAC and row scope.
//
// Rep3 is neither the requester nor the introducer on this ask. They could not
// read it, and no endpoint would let them answer it. But they hold
// activity:create, and the log path stamps a contact as `from` on any activity
// whose direction says inbound — with an occurred_at they choose. Without the
// provenance clause, one logged "email" closes an introduction between two
// colleagues who never heard from the contact at all.
//
// `source_system` cannot be the test: it is settable on the public log
// endpoint, so a body carrying `source_system: gmail` forges it in one line.
// `captured_by` comes from the authenticated principal and cannot be asserted
// by a caller, which is what makes it the honest one.
func TestAHandLoggedMessageCannotCloseAnIntroduction(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	fx.deliver(t, fx.messageCapturedBy(t, e, humanProvenance,
		"inbound", "from", fx.contact, fx.handshake.Add(time.Hour)))

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("a hand-logged message moved the ask to %q — a member who is not "+
			"party to this ask just closed it", got)
	}

	// The same message under a connector's provenance DOES close it, so the
	// refusal above is about who wrote the row and not about the fixture.
	fx.deliver(t, fx.messageCapturedBy(t, e, connectorProvenance,
		"inbound", "from", fx.contact, fx.handshake.Add(2*time.Hour)))
	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("a captured message left the ask at %q; want replied", got)
	}
}

// A reply captured BEFORE its sender was a contact is not lost.
//
// Capture writes the message and its address-only participant in one
// transaction, then promotes that address to a person in a second one
// afterwards (capture/sinkensure.go). The outbox relay can publish
// activity.captured in the gap, so the activity arm reads the message, finds no
// person on it, and acknowledges. Nothing would ever look at that message
// again: the reply is lost permanently and the ask keeps reading unanswered,
// which to the requester is indistinguishable from the contact ignoring them.
//
// The person arm is the repair. This drives the race in the order it actually
// happens — message first with no person on it, promotion second — and requires
// the ask to close.
func TestAReplyCapturedBeforeItsSenderExistsIsStillFound(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)
	sentAt := fx.handshake.Add(time.Hour)

	// The message as capture first writes it: no person on the participant row,
	// only the address it arrived from.
	activity := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: retrofit', $2, 'inbound', 'sync', $3)`,
		activity, sentAt, connectorProvenance)
	e.WsExec(t, `
		INSERT INTO activity_participant (activity_id, address, role)
		VALUES ($1, 'dana@northgate.test', 'from')`, activity)

	fx.deliver(t, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "activity.captured",
		Entity:  kevents.EntityRef{Type: "activity", ID: activity},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	})
	// Nothing yet, and that is correct rather than a fault: at this instant the
	// message names no contact.
	if got := fx.statusOf(t, e); got != "introduced" {
		t.Fatalf("the ask moved to %q before its sender was a contact", got)
	}

	// Promotion, which is the second transaction capture runs.
	e.WsExec(t, `
		UPDATE activity_participant SET person_id = $1
		 WHERE activity_id = $2 AND role = 'from'`, fx.contact, activity)
	fx.deliver(t, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "person.updated",
		Entity:  kevents.EntityRef{Type: "person", ID: fx.contact},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	})

	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("the ask is %q after its sender was promoted; want replied — "+
			"the reply captured before the contact existed was lost", got)
	}
}

// The repair arm admits exactly what the live arm admits.
//
// A backlog pass with a looser rule is worse than none: it would close, quietly
// and in bulk, the asks the live path had already refused. So the same three
// clauses are checked here through the person arm — a hand-logged message, an
// outbound one, and one predating the handshake all leave the ask alone.
func TestTheRepairArmRefusesWhatTheLiveArmRefuses(t *testing.T) {
	e := Setup(t)
	fx := seedIntroduced(t, e)

	// Hand-logged: the forgery case, reached through the repair path.
	fx.messageCapturedBy(t, e, humanProvenance,
		"inbound", "from", fx.contact, fx.handshake.Add(time.Hour))
	// Outbound: the rep's own follow-up.
	fx.message(t, e, "outbound", "from", fx.contact, fx.handshake.Add(time.Hour))
	// Older than the handshake: backfilled history.
	fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(-48*time.Hour))

	fx.deliver(t, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "person.updated",
		Entity:  kevents.EntityRef{Type: "person", ID: fx.contact},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	})

	if got := fx.statusOf(t, e); got != "introduced" {
		t.Errorf("the repair arm closed the ask on evidence the live arm refuses (%q)", got)
	}

	// And a qualifying message DOES close it through the same arm, so the
	// refusals above are about the evidence rather than a dead code path.
	fx.message(t, e, "inbound", "from", fx.contact, fx.handshake.Add(2*time.Hour))
	fx.deliver(t, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "person.updated",
		Entity:  kevents.EntityRef{Type: "person", ID: fx.contact},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	})
	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("the repair arm left a genuine reply at %q; want replied", got)
	}
}

// A reply that arrives while the handshake is still being recorded is found.
//
// The message can land before Complete commits. At that instant the ask is
// still `accepted`, nothing is awaiting a reply, and the activity arm correctly
// does nothing — but the reply is real and must not be lost for having been
// early.
//
// intro_request.completed rides on the PERSON entity, so the handshake
// committing publishes an event the person arm consumes. That is why this arm
// is keyed on the entity type rather than on person.created/updated alone: it
// is the same repair, reached by the event the handshake itself emits.
func TestAReplyThatBeatsTheHandshakeIsStillFound(t *testing.T) {
	e := Setup(t)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	store := introductions.NewStore(e.DB(), time.Now)
	requester := e.As(e.Rep1, nil, introPerms)
	introducer := e.As(e.Rep2, nil, introPerms)

	id, err := store.Create(requester, introductions.NewRequest{
		PersonID: contact, IntroducerUser: e.Rep2, RouteType: "direct",
		InternalReason: "Dana reopened the retrofit conversation.",
		DueAt:          time.Now().AddDate(0, 0, 7),
	})
	if err != nil {
		t.Fatalf("asking for the introduction: %v", err)
	}
	if err := store.Decide(introducer, id, introductions.StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("the colleague accepting: %v", err)
	}
	fx := introFixture{
		ask: id, contact: contact, store: store,
		advance: compose.NewIntroAdvance(e.Pool, store, slog.New(slog.DiscardHandler)),
	}

	// The reply lands while the ask is still only accepted.
	early := time.Now().Add(-time.Minute)
	fx.deliver(t, fx.message(t, e, "inbound", "from", contact, early))
	if got := fx.statusOf(t, e); got != "accepted" {
		t.Fatalf("an ask nobody had introduced yet moved to %q", got)
	}

	// Now the handshake commits, dated BEFORE that message so the reply
	// genuinely follows it.
	if err := store.Complete(introducer, id, nil, 2); err != nil {
		t.Fatalf("recording the handshake: %v", err)
	}
	e.WsExec(t, `UPDATE intro_request SET introduced_at = $2 WHERE id = $1`,
		id, early.Add(-time.Minute))

	// The event the handshake itself emits, on the person entity.
	fx.deliver(t, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "intro_request.completed",
		Entity:  kevents.EntityRef{Type: "person", ID: contact},
		Trace:   kevents.Trace{CorrelationID: ids.NewV7()},
	})

	if got := fx.statusOf(t, e); got != "replied" {
		t.Errorf("the ask is %q after the handshake caught up; want replied — "+
			"a reply that arrived first was lost", got)
	}
}
