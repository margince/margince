// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whether a mail is still owed a reply, asked of mail that came in through the
// production capture sink and the real mail parser.
//
// The answer check reads thread keys, recipient rows and counterparty columns
// exactly as capture writes them, so hand-inserted rows would test a shape
// capture might never produce. Each case reads both surfaces: the needs-reply
// badge (EmailSummariesByID) and the waiting lane (WaitingReplies).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// owedMail is one message as a mail program would write it. An empty
// inReplyTo starts a new thread, which is what a client that drops the reply
// headers does.
type owedMail struct {
	from, to, cc, bcc, subject, messageID, inReplyTo string
	at                                               time.Time
	// spoofed leaves out the provider's word that the seat sent it: a message
	// whose From merely names the seat's own address.
	spoofed bool
}

func (m owedMail) raw() []byte {
	lines := []string{"From: " + m.from, "To: " + m.to}
	if m.cc != "" {
		lines = append(lines, "Cc: "+m.cc)
	}
	if m.bcc != "" {
		lines = append(lines, "Bcc: "+m.bcc)
	}
	lines = append(lines, "Subject: "+m.subject, "Date: "+m.at.Format(time.RFC1123Z),
		"Message-ID: <"+m.messageID+">")
	if m.inReplyTo != "" {
		lines = append(lines, "In-Reply-To: <"+m.inReplyTo+">", "References: <"+m.inReplyTo+">")
	}
	lines = append(lines, "Content-Type: text/plain; charset=utf-8", "", "Text.", "")
	return []byte(strings.Join(lines, "\r\n"))
}

// owedEnv is one seat with a connected, shared mailbox, reading as that seat.
type owedEnv struct {
	e     *integration.Env
	owner ids.UUID
	seat  string
	now   time.Time
}

func setupOwed(t *testing.T) *owedEnv {
	t.Helper()
	e := integration.Setup(t)
	o := &owedEnv{e: e, owner: e.Rep1, seat: seatAddress(t, e, e.Rep1), now: time.Now()}
	// A shared mailbox, so captured mail is workspace mail: the classifier
	// judges only mail the workspace may read.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, o.owner)
		return err
	}); err != nil {
		t.Fatalf("sharing the mailbox: %v", err)
	}
	return o
}

// capture files one message through the production sink and returns its row.
func (o *owedEnv) capture(t *testing.T, m owedMail) ids.UUID {
	t.Helper()
	return o.captureFor(t, o.owner, o.seat, m)
}

// captureFor files the message into another seat's mailbox. A message from
// that seat carries the provider's sent attestation unless it is spoofed.
func (o *owedEnv) captureFor(t *testing.T, owner ids.UUID, seat string, m owedMail) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(m.raw(), seat)
	if err != nil {
		t.Fatalf("parsing %s: %v", m.messageID, err)
	}
	parsed = parsed.AttestSentByOwner(m.from == seat && !m.spoofed)
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(
		threadConnectorCtx(o.e, owner), parsed.ToRecord("gmail", m.raw()))
	if err != nil {
		t.Fatalf("capturing %s: %v", m.messageID, err)
	}
	return ref.ID
}

// contact creates a contact who writes from the address, so capture files
// their mail under them and names them on the sender row.
func (o *owedEnv) contact(t *testing.T, name string, addresses ...string) ids.UUID {
	t.Helper()
	emails := make([]contacts.ContactEmailInput, 0, len(addresses))
	for i, address := range addresses {
		emails = append(emails, contacts.ContactEmailInput{
			Email: address, EmailType: "work", IsPrimary: i == 0, Position: i,
		})
	}
	created, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Emails: emails,
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

// customerWrites captures an inbound mail from the address to the seat.
func (o *owedEnv) customerWrites(t *testing.T, from, subject string, at time.Time) ids.UUID {
	t.Helper()
	return o.capture(t, owedMail{
		from: from, to: o.seat, subject: subject, at: at,
		messageID: "in-" + ids.NewV7().String() + "@customer.example",
	})
}

// weWrite captures an outbound mail from the seat that starts its own thread.
func (o *owedEnv) weWrite(t *testing.T, m owedMail) ids.UUID {
	t.Helper()
	m.from = o.seat
	if m.messageID == "" {
		m.messageID = "out-" + ids.NewV7().String() + "@ws.example"
	}
	return o.capture(t, m)
}

// weLog records a call or a held meeting with the contact through the
// ordinary activity writer.
func (o *owedEnv) weLog(t *testing.T, kind string, contact ids.UUID, at time.Time) {
	t.Helper()
	subject := "Spoke about it"
	in := activities.LogActivityInput{
		Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	if kind == string(crmcontracts.ActivityKindMeeting) {
		held := string(crmcontracts.ActivityMeetingStatusHeld)
		in.MeetingStatus = &held
	}
	if _, _, err := o.e.Activities.LogActivity(o.e.Admin(), in); err != nil {
		t.Fatalf("logging a %s: %v", kind, err)
	}
}

// count runs one counting query as the admin, for a fixture check.
func (o *owedEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

// badged reports whether the needs-reply badge is on the message.
func (o *owedEnv) badged(t *testing.T, message ids.UUID) bool {
	t.Helper()
	summaries, err := activities.NewStore(o.e.DB()).EmailSummariesByID(o.reader(), []ids.UUID{message})
	if err != nil {
		t.Fatalf("reading the badge: %v", err)
	}
	summary, ok := summaries[message]
	if !ok {
		t.Fatalf("no email row came back for %s", message)
	}
	return summary.Move == crmcontracts.EmailSummaryMoveNeedsReply
}

// waiting reports whether the waiting lane shows the message.
func (o *owedEnv) waiting(t *testing.T, message ids.UUID) bool {
	t.Helper()
	rows, err := activities.NewStore(o.e.DB()).WaitingReplies(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the waiting lane: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == message {
			return true
		}
	}
	return false
}

// owed asserts the badge and the lane agree, and returns their answer.
func (o *owedEnv) owed(t *testing.T, message ids.UUID) bool {
	t.Helper()
	badge, lane := o.badged(t, message), o.waiting(t, message)
	if badge != lane {
		t.Fatalf("the badge says owed=%v and the lane says owed=%v about the same message", badge, lane)
	}
	return badge
}

func (o *owedEnv) threadKey(t *testing.T, message ids.UUID) string {
	t.Helper()
	var key string
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, message).Scan(&key)
	}); err != nil {
		t.Fatalf("reading the thread key: %v", err)
	}
	return key
}

func (o *owedEnv) reader() context.Context {
	return o.e.As(o.owner, nil, integration.AdminPerms)
}

func (o *owedEnv) classifier() principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:owed_verdict",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	}
}

// A reply that lost its thread still answers: a new thread key, "Re:" and the
// sender's address. Capture keeps the two on separate threads.
func TestAReplyThatLostItsThreadAnswersTheMail(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Invoice March", o.now.Add(-3*time.Hour))
	if !o.owed(t, asked) {
		t.Fatal("an unanswered, unjudged mail is not owed; the cases below would pass for the wrong reason")
	}

	reply := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "RE: AW:  Invoice March", at: o.now.Add(-2 * time.Hour),
	})

	if o.threadKey(t, reply) == o.threadKey(t, asked) {
		t.Fatal("capture joined the reply to the thread on its subject; the fixture no longer loses its thread")
	}
	if o.owed(t, asked) {
		t.Fatal("a reply to the sender with the same subject did not answer the mail")
	}
}

// What does not answer: another subject, a forward, a copy the sender cannot
// see, and a reply to somebody else.
func TestOnlyASameSubjectReplyToTheSenderAnswers(t *testing.T) {
	cases := map[string]func(o *owedEnv, subject string){
		"another subject": func(o *owedEnv, _ string) {
			o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Something else", at: o.now.Add(-time.Hour)})
		},
		"a forward": func(o *owedEnv, subject string) {
			o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Fwd: " + subject, at: o.now.Add(-time.Hour)})
		},
		"a blind copy": func(o *owedEnv, subject string) {
			o.weWrite(t, owedMail{
				to: "desk@elsewhere.example", bcc: "pat@customer.example",
				subject: "Re: " + subject, at: o.now.Add(-time.Hour),
			})
		},
		"a reply to somebody else": func(o *owedEnv, subject string) {
			o.weWrite(t, owedMail{to: "sam@customer.example", subject: "Re: " + subject, at: o.now.Add(-time.Hour)})
		},
	}
	for name, answerAttempt := range cases {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			subject := "Quote " + ids.NewV7().String()[:8]
			asked := o.customerWrites(t, "pat@customer.example", subject, o.now.Add(-3*time.Hour))
			answerAttempt(o, subject)
			if !o.owed(t, asked) {
				t.Fatalf("%s answered the mail", name)
			}
		})
	}
}

// A copy the sender CAN see answers: Cc is a recipient row.
func TestAReplyCopyingTheSenderAnswers(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Contract draft", o.now.Add(-3*time.Hour))
	o.weWrite(t, owedMail{
		to: "legal@customer.example", cc: "pat@customer.example",
		subject: "Re: Contract draft", at: o.now.Add(-time.Hour),
	})
	if o.owed(t, asked) {
		t.Fatal("a reply copying the sender did not answer the mail")
	}
}

// Two "Re: Invoice" mails from two senders are two conversations: answering
// one leaves the other owed.
func TestTwoSameSubjectMailsFromTwoSendersNeverClearEachOther(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	o.contact(t, "Robin Buyer", "robin@other.example")
	pat := o.customerWrites(t, "pat@customer.example", "Re: Invoice", o.now.Add(-4*time.Hour))
	robin := o.customerWrites(t, "robin@other.example", "Re: Invoice", o.now.Add(-3*time.Hour))
	if o.threadKey(t, pat) == o.threadKey(t, robin) {
		t.Fatal("capture joined two senders' mail on a shared subject")
	}

	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Invoice", at: o.now.Add(-2 * time.Hour)})

	if o.owed(t, pat) {
		t.Error("the reply to Pat did not answer Pat")
	}
	if !o.owed(t, robin) {
		t.Error("the reply to Pat answered Robin, who wrote the same subject")
	}
}

// A call or a held meeting with the sender answers them. The same with a
// colleague of theirs does not.
func TestACallOrHeldMeetingWithTheSenderAnswers(t *testing.T) {
	for _, kind := range []string{string(crmcontracts.ActivityKindCall), string(crmcontracts.ActivityKindMeeting)} {
		t.Run(kind, func(t *testing.T) {
			o := setupOwed(t)
			pat := o.contact(t, "Pat Buyer", "pat@customer.example")
			sam := o.contact(t, "Sam Buyer", "sam@customer.example")
			asked := o.customerWrites(t, "pat@customer.example", "Can we talk", o.now.Add(-3*time.Hour))

			o.weLog(t, kind, sam, o.now.Add(-2*time.Hour))
			if !o.owed(t, asked) {
				t.Fatalf("a %s with the sender's colleague answered the sender", kind)
			}
			o.weLog(t, kind, pat, o.now.Add(-time.Hour))
			if o.owed(t, asked) {
				t.Fatalf("a %s with the sender did not answer them", kind)
			}
		})
	}
}

// Mail judged to ask nothing leaves the lane and the badge, and is counted and
// listed under informs_us. A request a human accepted stays, whatever the
// verdict.
func TestMailJudgedToAskNothingIsHiddenWithItsFigure(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	report := o.customerWrites(t, "pat@customer.example", "Monthly report", o.now.Add(-3*time.Hour))
	accepted := o.customerWrites(t, "pat@customer.example", "Please call me", o.now.Add(-2*time.Hour))
	store := activities.NewStore(o.e.DB())
	before, err := store.HiddenWaiting(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	if _, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindTask), Source: "ui", RequestActivityID: &accepted,
	}); err != nil {
		t.Fatalf("accepting the request: %v", err)
	}
	judge := principal.WithActor(o.e.Admin(), o.classifier())
	for _, message := range []ids.UUID{report, accepted} {
		applied, err := o.e.Activities.SetOwedVerdict(judge, message, activities.OwedVerdictInformsUs,
			"prompts-test", time.Now())
		if err != nil || !applied {
			t.Fatalf("judging: applied=%v err=%v", applied, err)
		}
	}

	if o.owed(t, report) {
		t.Error("mail judged to ask nothing is still owed")
	}
	// The badge alone: the lane leaves an accepted request to the reminder in
	// the acceptor's own task list, which is a queue rule and not the verdict.
	if !o.badged(t, accepted) {
		t.Error("a request a human accepted was hidden by the model's verdict")
	}
	after, err := store.HiddenWaiting(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	if got := after.InformsUs - before.InformsUs; got != 1 {
		t.Errorf("informs_us counted %d more, want the one report", got)
	}
	rows, err := store.HiddenWaitingRows(o.reader(), time.Now(), activities.HiddenRuleInformsUs)
	if err != nil {
		t.Fatalf("listing informs_us: %v", err)
	}
	listed := map[ids.UUID]bool{}
	for _, row := range rows {
		listed[row.ActivityID] = true
	}
	if !listed[report] || listed[accepted] {
		t.Errorf("informs_us lists report=%v accepted=%v, want only the report", listed[report], listed[accepted])
	}
}

// The badge and the lane agree on every message of a mixed mailbox. The one
// difference allowed is a queue rule, and each such rule has its figure in
// /worklist/hidden: here, mail past the horizon is still owed on its badge,
// absent from the lane, and counted as past_horizon.
func TestTheBadgeAndTheLaneAgreeOnEveryOwedMail(t *testing.T) {
	o := setupOwed(t)
	// One sender per answered case: a call with a sender answers every earlier
	// mail of theirs, so a shared sender would let one answer mask another.
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	o.contact(t, "Robin Buyer", "robin@other.example")
	o.contact(t, "Casey Buyer", "casey@third.example")
	o.contact(t, "Drew Buyer", "drew@fourth.example")
	judge := principal.WithActor(o.e.Admin(), o.classifier())
	at := func(hoursAgo int) time.Time { return o.now.Add(-time.Duration(hoursAgo) * time.Hour) }

	unanswered := o.customerWrites(t, "robin@other.example", "Pricing question", at(9))
	forwarded := o.customerWrites(t, "robin@other.example", "Delivery date", at(9))
	o.weWrite(t, owedMail{to: "robin@other.example", subject: "Fwd: Delivery date", at: at(8)})

	onThread := o.capture(t, owedMail{
		from: "casey@third.example", to: o.seat, subject: "Workshop",
		messageID: "workshop@third.example", at: at(9),
	})
	// Renamed on the way back, so only the thread says it is the reply.
	o.weWrite(t, owedMail{
		to: "casey@third.example", subject: "Agenda for Tuesday",
		inReplyTo: "workshop@third.example", at: at(8),
	})
	bySubject := o.customerWrites(t, "drew@fourth.example", "Invoice April", at(7))
	o.weWrite(t, owedMail{to: "drew@fourth.example", subject: "Re: Invoice April", at: at(6)})
	byCall := o.customerWrites(t, "pat@customer.example", "Call me", at(5))
	o.weLog(t, string(crmcontracts.ActivityKindCall), pat, at(4))

	request := o.customerWrites(t, "robin@other.example", "Send the contract", at(5))
	if applied, err := o.e.Activities.SetOwedVerdict(judge, request, activities.OwedVerdictAsksUs,
		"prompts-test", time.Now()); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}
	o.weWrite(t, owedMail{to: "robin@other.example", subject: "Re: Send the contract", at: at(4)})
	report := o.customerWrites(t, "robin@other.example", "Weekly figures", at(3))
	if applied, err := o.e.Activities.SetOwedVerdict(judge, report, activities.OwedVerdictInformsUs,
		"prompts-test", time.Now()); err != nil || !applied {
		t.Fatalf("judging the report: applied=%v err=%v", applied, err)
	}

	store := activities.NewStore(o.e.DB())
	before, err := store.HiddenWaiting(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	old := o.customerWrites(t, "robin@other.example", "Spring order", o.now.AddDate(0, 0, -150))

	for name, want := range map[string]struct {
		message ids.UUID
		owed    bool
	}{
		"unanswered":              {unanswered, true},
		"forwarded only":          {forwarded, true},
		"answered on the thread":  {onThread, false},
		"answered by subject":     {bySubject, false},
		"answered by a call":      {byCall, false},
		"a request we replied to": {request, true},
		"judged to ask nothing":   {report, false},
	} {
		if got := o.owed(t, want.message); got != want.owed {
			t.Errorf("%s: owed=%v, want %v", name, got, want.owed)
		}
	}

	if !o.badged(t, old) || o.waiting(t, old) {
		t.Fatalf("mail past the horizon: badge=%v lane=%v, want owed on the badge and off the lane",
			o.badged(t, old), o.waiting(t, old))
	}
	after, err := store.HiddenWaiting(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	if got := after.PastHorizon - before.PastHorizon; got != 1 {
		t.Errorf("past_horizon counted %d more, want the one old mail the lane leaves out", got)
	}
}

// A reply sent from Margince itself records its recipient on the outbound
// row only, with no recipient rows. It still answers a mail whose thread it
// lost, whatever casing the caller gave the recipient.
func TestAReplySentFromMarginceAnswersThroughItsCounterparty(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Renewal", o.now.Add(-3*time.Hour))
	subject, body, direction := "Re: Renewal", "Here it is.", "outbound"
	system, messageID, at := "email", "sent-"+ids.NewV7().String()+"@ws.example", o.now.Add(-time.Hour)
	sent, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at,
		Source: "manual", SourceSystem: &system, SourceID: &messageID, ThreadKey: messageID,
		CounterpartyEmail: "Pat@Customer.EXAMPLE", CounterpartyOutboundAttested: true,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: pat}},
	})
	if err != nil {
		t.Fatalf("recording the sent reply: %v", err)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address IS NOT NULL`, sent.Id); n != 0 {
		t.Fatalf("the sent reply carries %d recipient row(s); the case needs the counterparty alone", n)
	}
	if o.owed(t, asked) {
		t.Fatal("a reply sent from Margince to the sender did not answer them")
	}
}

// The response time is measured to the same answer: a reply that lost its
// thread counts as answered, at the time it was sent.
func TestTheResponseTimeCountsAReplyThatLostItsThread(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	store := activities.NewStore(o.e.DB())
	from, to := o.now.Add(-24*time.Hour), o.now.Add(time.Hour)
	before, err := store.ResponseWindow(o.reader(), from, to)
	if err != nil {
		t.Fatalf("reading the response window: %v", err)
	}
	o.customerWrites(t, "pat@customer.example", "Invoice May", o.now.Add(-3*time.Hour))
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Invoice May", at: o.now.Add(-2 * time.Hour)})

	after, err := store.ResponseWindow(o.reader(), from, to)
	if err != nil {
		t.Fatalf("reading the response window: %v", err)
	}
	if got := after.Answered - before.Answered; got != 1 {
		t.Fatalf("the window counted %d more answered, want the one answered by a reply that lost its thread", got)
	}
}

// A colleague's private reply, and a private meeting, answer nothing for
// another seat: the row going quiet would disclose that they exist. The same
// evidence the whole workspace can read does answer.
func TestPrivateEvidenceDoesNotAnswerAnotherSeatsMail(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Invoice June", o.now.Add(-4*time.Hour))
	colleague := o.e.Rep2
	colleagueSeat := seatAddress(t, o.e, colleague)
	private := o.captureFor(t, colleague, colleagueSeat, owedMail{
		from: colleagueSeat, to: "pat@customer.example", subject: "Re: Invoice June",
		messageID: "private-" + ids.NewV7().String() + "@ws.example", at: o.now.Add(-3 * time.Hour),
	})
	if n := o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'workspace'`, private); n != 0 {
		t.Fatal("the colleague's mailbox shares its mail; the case needs a private reply")
	}
	if !o.owed(t, asked) {
		t.Fatal("a colleague's private reply answered another seat's mail")
	}

	subject, held := "Met Pat", string(crmcontracts.ActivityMeetingStatusHeld)
	at, author := o.now.Add(-2*time.Hour), o.e.As(colleague, nil, integration.AdminPerms)
	meeting, _, err := o.e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &at, Source: "manual",
		MeetingStatus: &held, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: pat}},
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	if _, err := o.e.Activities.SetAudience(author, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("making the meeting private: %v", err)
	}
	if !o.owed(t, asked) {
		t.Fatal("a colleague's private meeting answered another seat's mail")
	}

	o.weLog(t, string(crmcontracts.ActivityKindCall), pat, o.now.Add(-time.Hour))
	if o.owed(t, asked) {
		t.Fatal("a call the whole workspace can read did not answer the mail")
	}
}

// A message whose From names our mailbox but that the provider never filed as
// sent answers nothing. The attested reply does.
func TestASpoofedReplyAnswersNothing(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Bank details", o.now.Add(-3*time.Hour))

	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Bank details", at: o.now.Add(-2 * time.Hour), spoofed: true})
	if !o.owed(t, asked) {
		t.Fatal("a spoofed reply answered the mail")
	}
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Bank details", at: o.now.Add(-time.Hour)})
	if o.owed(t, asked) {
		t.Fatal("the attested reply did not answer the mail")
	}
}

// The sender is their contact, not one address: a reply to another address
// of the same contact answers. Capture still files the two on two threads.
func TestAReplyToAnotherAddressOfTheSenderAnswers(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example", "pat.private@home.example")
	asked := o.customerWrites(t, "pat@customer.example", "Offer", o.now.Add(-3*time.Hour))
	reply := o.weWrite(t, owedMail{to: "pat.private@home.example", subject: "Re: Offer", at: o.now.Add(-time.Hour)})
	if o.threadKey(t, reply) == o.threadKey(t, asked) {
		t.Fatal("capture joined the two on their subject; the fixture no longer loses its thread")
	}
	if o.owed(t, asked) {
		t.Fatal("a reply to another address of the same contact did not answer them")
	}
}

// A reply in the same second as the mail is no evidence of an answer, in
// either capture order: an id says when a row was captured, not which message
// came first.
func TestASameSecondReplyIsNotAnAnswer(t *testing.T) {
	for _, replyFirst := range []bool{false, true} {
		name := map[bool]string{false: "mail captured first", true: "reply captured first"}[replyFirst]
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			second := o.now.Add(-time.Hour).Truncate(time.Second)
			reply := func() {
				o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Samples", at: second})
			}
			if replyFirst {
				reply()
			}
			asked := o.customerWrites(t, "pat@customer.example", "Samples", second)
			if !replyFirst {
				reply()
			}
			if !o.owed(t, asked) {
				t.Fatal("a reply in the same second counted as an answer")
			}
		})
	}
}
