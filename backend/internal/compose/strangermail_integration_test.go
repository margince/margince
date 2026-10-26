// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A stranger's Bcc or foreign-link mail ranks as routine work with its reason,
// even when the classifier read it as a request. The same mail from a known
// sender keeps the top band. Every message goes through the capture sink, the
// real parser, the waiting lane and the Worklist.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The Worklist's top band and its routine band, as the attention package
// numbers them.
const (
	worklistTopBand     = 1
	worklistRoutineBand = 6
)

// redirectedLink is a call-to-action wrapped in Google's link redirect, whose
// real target is a site that is neither the sender's nor ours.
const redirectedLink = `<p><a href="https://www.google.com/url?q=https://landing.example.org/doc/index.html&amp;sa=D">View the document</a></p>`

// strangerMail is a mail that reached the seat only by Bcc.
func (o *owedEnv) strangerMail(from, subject string, at time.Time) owedMail {
	return owedMail{
		from: from, to: "undisclosed-recipients:;", deliveredTo: o.seat, subject: subject, at: at,
		messageID: "in-" + ids.NewV7().String() + "@mail.example",
	}
}

// judgeAsRequest records the classifier's asks-us verdict on a message.
func (o *owedEnv) judgeAsRequest(t *testing.T, message ids.UUID) {
	t.Helper()
	judge := principal.WithActor(o.e.Admin(), o.classifier())
	applied, err := o.e.Activities.SetOwedVerdict(judge, message, activities.OwedVerdictAsksUs, "prompts-test", o.now)
	if err != nil || !applied {
		t.Fatalf("judging %s as a request: applied=%v err=%v", message, applied, err)
	}
}

// requestFrom captures one mail into the seat's mailbox and judges it a request.
func (o *owedEnv) requestFrom(t *testing.T, m owedMail) ids.UUID {
	t.Helper()
	if m.messageID == "" {
		m.messageID = "in-" + ids.NewV7().String() + "@mail.example"
	}
	message := o.capture(t, m)
	o.fileUnderSender(t, message, m.from)
	o.judgeAsRequest(t, message)
	return message
}

// fileUnderSender gives a stranger's mail the contact it waits on. Capture
// files mail only under a contact that already exists, and a stranger has
// none until somebody makes one after the mail arrived.
func (o *owedEnv) fileUnderSender(t *testing.T, message ids.UUID, from string) {
	t.Helper()
	if o.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id IS NOT NULL`, message) > 0 {
		return
	}
	contact := o.contact(t, "Sam Stranger", from)
	if _, err := o.e.Activities.RelinkActivity(o.e.Admin(), ids.From[ids.ActivityKind](message),
		activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
		t.Fatalf("filing the mail under its sender: %v", err)
	}
}

// worklistRow reads one reader's Worklist and returns the row for one message.
func (o *owedEnv) worklistRow(reader context.Context, t *testing.T, message ids.UUID) crmcontracts.WorklistItem {
	t.Helper()
	feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return o.now })
	page, err := feed.Worklist(reader, "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	for _, row := range page.Queue {
		if row.Id == message.String() {
			return row
		}
	}
	t.Fatalf("message %s is not on the Worklist; it must be demoted, never hidden", message)
	return crmcontracts.WorklistItem{}
}

func says(row crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range row.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}

// knownSender creates a contact the workspace wrote to a month ago.
func (o *owedEnv) knownSender(t *testing.T, address string) {
	t.Helper()
	o.contact(t, "Kim Known", address)
	o.weWrite(t, owedMail{to: address, subject: "Our offer", at: o.now.Add(-30 * 24 * time.Hour)})
}

// connectColleague connects a second seat's shared mailbox and answers its
// address. seatAddress names a seat by the first characters of its id.
// Two seats made in one second share them, so the colleague gets its own.
func (o *owedEnv) connectColleague(t *testing.T, seat ids.UUID) string {
	t.Helper()
	address := "colleague@ws.example"
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label, mail_posture)
			VALUES ($1, 'gmail', 'connected', 'vault:test', $2, 'shared')
			ON CONFLICT (user_id, provider) DO UPDATE SET account_label = $2, mail_posture = 'shared', archived_at = NULL`,
			seat, address)
		return err
	}); err != nil {
		t.Fatalf("connecting the colleague's mailbox: %v", err)
	}
	return address
}

func TestAStrangersBccRequestIsRoutineWhileAKnownSendersStaysOnTop(t *testing.T) {
	o := setupOwed(t)
	o.knownSender(t, "known@example.net")
	eightDaysAgo := o.now.Add(-8 * 24 * time.Hour)

	strangerCopy := o.strangerMail("sender@example.com", "View the document", eightDaysAgo)
	stranger := o.requestFrom(t, strangerCopy)
	known := o.requestFrom(t, o.strangerMail("known@example.net", "Next steps", eightDaysAgo))

	// A replay after the sender became a contact keeps what the first capture judged.
	o.capture(t, strangerCopy)
	if !o.strangerFact(t, stranger) ||
		o.count(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND bcc_only`, stranger) != 1 {
		t.Fatal("a replay overwrote the facts the first capture judged")
	}

	// Copied by Bcc with no seat in To, neither request is anybody's duty, so
	// neither becomes shared work. A Bcc seat beside a seat named in To is
	// TestBccIsJudgedForEachSeatWhicheverCapturesFirst.
	for name, message := range map[string]ids.UUID{"the stranger's": stranger, "the known sender's": known} {
		if o.onWorklist(o.reader(), t, message) {
			t.Errorf("%s Bcc request with no seat in To is on the team Worklist", name)
		}
	}
}

// onWorklist reports whether the message is a row of the team Worklist.
func (o *owedEnv) onWorklist(reader context.Context, t *testing.T, message ids.UUID) bool {
	t.Helper()
	feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return o.now })
	page, err := feed.Worklist(reader, "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	for _, row := range page.Queue {
		if row.Id == message.String() {
			return true
		}
	}
	return false
}

// Bcc is a fact per seat. One seat named in To and one copied by Bcc each see
// their own answer, whichever mailbox captured the message first.
func TestBccIsJudgedForEachSeatWhicheverCapturesFirst(t *testing.T) {
	for _, colleagueFirst := range []bool{false, true} {
		name := map[bool]string{false: "the addressed seat first", true: "the Bcc seat first"}[colleagueFirst]
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			colleague := o.e.Rep2
			colleagueSeat := o.connectColleague(t, colleague)
			addressed := owedMail{
				from: "sender@example.com", to: o.seat, deliveredTo: o.seat, subject: "View the document",
				at: o.now.Add(-2 * 24 * time.Hour), messageID: "in-" + ids.NewV7().String() + "@mail.example",
			}
			copied := addressed
			copied.deliveredTo = colleagueSeat

			var message ids.UUID
			if colleagueFirst {
				message = o.captureFor(t, colleague, colleagueSeat, copied)
				o.captureFor(t, o.owner, o.seat, addressed)
			} else {
				message = o.captureFor(t, o.owner, o.seat, addressed)
				o.captureFor(t, colleague, colleagueSeat, copied)
			}
			o.fileUnderSender(t, message, addressed.from)
			o.judgeAsRequest(t, message)

			if row := o.worklistRow(o.reader(), t, message); row.Level != worklistTopBand || says(row, "bcc_from_stranger") {
				t.Errorf("the seat named in To sees level %d with reasons %v, want the top band", row.Level, row.Because)
			}
			colleagueReader := o.e.As(colleague, nil, integration.AdminPerms)
			if row := o.worklistRow(colleagueReader, t, message); row.Level != worklistRoutineBand || !says(row, "bcc_from_stranger") {
				t.Errorf("the Bcc seat sees level %d with reasons %v, want routine with bcc_from_stranger", row.Level, row.Because)
			}
		})
	}
}

func TestAStrangersRequestWithAForeignLinkIsRoutineWhileAPlainOneStaysOnTop(t *testing.T) {
	o := setupOwed(t)
	o.knownSender(t, "known@example.net")
	twoDaysAgo := o.now.Add(-2 * 24 * time.Hour)
	toSeat := func(from, subject, html string) owedMail {
		return owedMail{from: from, to: o.seat, subject: subject, at: twoDaysAgo, html: html}
	}

	foreign := o.requestFrom(t, toSeat("sender@example.com", "View the document", redirectedLink))
	ownSite := o.requestFrom(t, toSeat("lee@example.com", "Our brochure",
		`<a href="https://docs.example.com/brochure">Brochure</a>`))
	known := o.requestFrom(t, toSeat("known@example.net", "The document", redirectedLink))

	row := o.worklistRow(o.reader(), t, foreign)
	if row.Level != worklistRoutineBand || !says(row, "link_off_sender") || says(row, "bcc_from_stranger") {
		t.Errorf("the stranger's foreign-link request is at level %d with reasons %v, want routine with only link_off_sender",
			row.Level, row.Because)
	}
	for name, message := range map[string]ids.UUID{
		"a stranger's link to their own site":             ownSite,
		"a stranger's link to their company's other site": o.companySiteRequest(t, twoDaysAgo),
		"a known sender's foreign link":                   known,
	} {
		if row := o.worklistRow(o.reader(), t, message); row.Level != worklistTopBand {
			t.Errorf("%s left the top band: level %d, reasons %v", name, row.Level, row.Because)
		}
	}
}

// companySiteRequest is a stranger's request linking to the second domain of
// the company that holds the sender's own domain.
func (o *owedEnv) companySiteRequest(t *testing.T, at time.Time) ids.UUID {
	t.Helper()
	if _, err := o.e.Contacts.CreateCompany(o.e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Two Sites", Source: "manual",
		Domains: []contacts.CompanyDomainInput{
			{Domain: "twosites.example", IsPrimary: true}, {Domain: "twosites-docs.example"},
		},
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	return o.requestFrom(t, owedMail{
		from: "ana@twosites.example", to: o.seat, subject: "Our paper",
		at: at, html: `<a href="https://twosites-docs.example/paper">Paper</a>`,
	})
}

// strangerFact reads what capture stored about the sender of one message.
func (o *owedEnv) strangerFact(t *testing.T, message ids.UUID) bool {
	t.Helper()
	return o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND sender_was_stranger`, message) == 1
}

// A live contact is known, though the workspace never wrote to them. A contact
// and a reply made after a mail arrived do not make that earlier mail known.
func TestWhoCountsAsAStrangerIsJudgedAtTheMailsArrival(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Contact", "pat@example.com")
	// Dated after the contact was made, which is after o.now.
	contact := o.requestFrom(t, o.strangerMail("pat@example.com", "Hello", o.now.Add(time.Hour)))
	if o.strangerFact(t, contact) {
		t.Error("a live contact's mail was recorded as a stranger's")
	}

	// A contact capture minted from the sender's first mail leaves them a stranger.
	if _, err := o.e.Contacts.CreateContact(threadConnectorCtx(o.e, o.owner), contacts.CreateContactInput{
		FullName: "Rae Repeat", Source: "gmail",
		Emails: []contacts.ContactEmailInput{{Email: "rae@example.com", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("minting the contact as capture does: %v", err)
	}
	if n := o.count(t, `SELECT count(*) FROM contact WHERE full_name = 'Rae Repeat' AND captured_by LIKE 'connector:%'`); n != 1 {
		t.Fatal("the minted contact does not carry capture's provenance; the case needs it")
	}
	if second := o.requestFrom(t, o.strangerMail("rae@example.com", "Again", o.now.Add(time.Hour))); !o.strangerFact(t, second) {
		t.Error("a contact capture minted from a stranger's mail made their next mail known")
	}

	eightDaysAgo := o.now.Add(-8 * 24 * time.Hour)
	o.contact(t, "Lou Later", "lou@example.com")
	if laterContact := o.requestFrom(t, o.strangerMail("lou@example.com", "First note", eightDaysAgo)); !o.strangerFact(t, laterContact) {
		t.Error("a contact made after the mail arrived made the mail look known")
	}
	o.weWrite(t, owedMail{to: "max@example.com", subject: "Following up", at: o.now.Add(-24 * time.Hour)})
	if laterReply := o.requestFrom(t, o.strangerMail("max@example.com", "First note", eightDaysAgo)); !o.strangerFact(t, laterReply) {
		t.Error("a reply written after the mail arrived made the mail look known")
	}
}

// A replay keeps the Bcc fact the seat's first capture judged, even when the
// seat has since declared the address the mail was written to.
func TestAReplayKeepsTheFirstBccJudgement(t *testing.T) {
	o := setupOwed(t)
	m := o.strangerMail("sender@example.com", "View the document", o.now.Add(-2*time.Hour))
	m.to = "old-alias@ws.example"
	message := o.capture(t, m)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
			VALUES ($1, 'address', 'old-alias@ws.example', 'test')`, o.owner)
		return err
	}); err != nil {
		t.Fatalf("declaring the alias: %v", err)
	}

	o.capture(t, m)

	if o.count(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND bcc_only`,
		message, o.owner) != 1 {
		t.Error("a replay overwrote the seat's first Bcc judgement")
	}
}

// Mail captured before these facts existed carries NULL, and NULL is no
// evidence: such mail ranks as it did.
func TestMailWithNoCapturedFactsRanksAsBefore(t *testing.T) {
	o := setupOwed(t)
	addressed := o.strangerMail("sender@example.com", "View the document", o.now.Add(-2*24*time.Hour))
	addressed.to = o.seat
	stranger := o.requestFrom(t, addressed)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`UPDATE activity SET foreign_link = NULL, sender_was_stranger = NULL WHERE id = $1`, stranger); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `UPDATE capture_import SET bcc_only = NULL WHERE activity_id = $1`, stranger)
		return err
	}); err != nil {
		t.Fatalf("clearing the captured facts: %v", err)
	}

	if row := o.worklistRow(o.reader(), t, stranger); row.Level != worklistTopBand {
		t.Errorf("mail with no captured facts is at level %d with reasons %v, want the top band", row.Level, row.Because)
	}
}

// A history import reads a mailbox newest first, so a stranger's mail can land
// before the reply the seat sent ahead of it. That reply still makes the mail
// known. A reply sent after the mail arrived does not.
func TestAReplyCapturedAfterTheMailItPrecededMakesItsSenderKnown(t *testing.T) {
	o := setupOwed(t)
	arrived := o.now.Add(-2 * 24 * time.Hour)
	answeredEarlier := o.capture(t, o.strangerMail("sender@example.com", "Following up", arrived))
	answeredLater := o.capture(t, o.strangerMail("late@example.com", "First note", arrived))
	if !o.strangerFact(t, answeredEarlier) || !o.strangerFact(t, answeredLater) {
		t.Fatal("mail captured before any reply was not recorded as a stranger's; the case needs it")
	}

	o.weWrite(t, owedMail{to: "sender@example.com", subject: "Our offer", at: arrived.Add(-24 * time.Hour)})
	o.weWrite(t, owedMail{to: "late@example.com", subject: "Thanks for writing", at: arrived.Add(time.Hour)})

	if o.strangerFact(t, answeredEarlier) {
		t.Error("a reply sent before the mail arrived, captured after it, left the sender a stranger")
	}
	if !o.strangerFact(t, answeredLater) {
		t.Error("a reply sent after the mail arrived made the sender of that mail known")
	}
}

// A To header that will not parse may have named the seat, so capture records
// no Bcc for it.
func TestAnUnreadableToLeavesTheBccQuestionOpen(t *testing.T) {
	o := setupOwed(t)
	m := o.strangerMail("sender@example.com", "View the document", o.now.Add(-2*time.Hour))
	m.to = "Reader <" + o.seat
	message := o.capture(t, m)

	if n := o.count(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND bcc_only IS NULL`,
		message, o.owner); n != 1 {
		t.Errorf("a mail whose To would not parse has %d import rows with no Bcc judgement, want 1", n)
	}
}
