// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A rep forwards a customer's mail to their own address. Through the
// production sink the forward is filed under the customer. It is marked as a
// forward nobody verified, and it stays the rep's own message. The original's
// From names whom it is about, never who wrote it.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const forwardedCustomer = "bea@prospect.example"

// forwardedOriginal is the mail a customer sent, which a forward carries.
type forwardedOriginal struct{ from, id string }

func customerOriginal(from, id string) forwardedOriginal {
	return forwardedOriginal{from: from, id: id}
}

func (m forwardedOriginal) raw() string {
	return strings.Join([]string{
		"From: Bea Kunde <" + m.from + ">", "To: " + captureOwner, "Subject: Angebot",
		"Date: Wed, 03 Jun 2026 09:00:00 +0000", "Message-ID: <" + m.id + ">",
		"Content-Type: text/plain; charset=utf-8", "", "please send the offer",
	}, "\r\n")
}

// forwardTo is the mail self sends to to, carrying original as a
// message/rfc822 attachment beside a file of its own. Like Gmail's, it names
// the original in References.
func forwardTo(self, to, msgID string, original forwardedOriginal) []byte {
	return []byte(strings.Join([]string{
		"From: " + self, "To: " + to, "Subject: Fwd: Angebot",
		"Date: Thu, 04 Jun 2026 08:00:00 +0000", "Message-ID: <" + msgID + ">", "References: <" + original.id + ">",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="F"`, "",
		"--F", "Content-Type: text/plain; charset=utf-8", "", "for the CRM",
		"--F", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="offer.pdf"`, "", "%PDF-1.4 offer",
		"--F", "Content-Type: message/rfc822", `Content-Disposition: attachment; filename="original.eml"`, "",
		original.raw(),
		"--F--", "",
	}, "\r\n"))
}

// filedForward is what one forward became.
type filedForward struct {
	id                                          ids.UUID
	completeness, counterparty, thread, contact string
	contactOwner                                ids.UUID
}

// adminRow scans one row of query into dest.
func adminRow(e *integration.SearchEnv, query string, args []any, dest ...any) error {
	return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(dest...)
	})
}

func forwardOf(t *testing.T, e *integration.SearchEnv, sourceID string) filedForward {
	t.Helper()
	var out filedForward
	err := adminRow(e, `
		SELECT a.id, a.body_completeness, a.counterparty_email, a.thread_key, c.full_name, c.owner_id
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
		  JOIN contact c ON c.id = l.contact_id
		  JOIN contact_email ce ON ce.contact_id = c.id AND ce.email = $2
		 WHERE a.source_id = $1`, []any{sourceID, forwardedCustomer},
		&out.id, &out.completeness, &out.counterparty, &out.thread, &out.contact, &out.contactOwner)
	if err != nil {
		t.Fatalf("reading the forward %s filed under %s: %v", sourceID, forwardedCustomer, err)
	}
	return out
}

// authorOf is the seat the message's From row names.
func authorOf(t *testing.T, e *integration.SearchEnv, activity ids.UUID) ids.UUID {
	t.Helper()
	var author ids.UUID
	if err := adminRow(e, `SELECT user_id FROM activity_participant WHERE activity_id = $1 AND role = 'from'`,
		[]any{activity}, &author); err != nil {
		t.Fatalf("reading who wrote %s: %v", activity, err)
	}
	return author
}

func TestAForwardToSelfIsFiledUnderTheCustomerAsTheRepsOwnMail(t *testing.T) {
	t.Parallel()
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e := env.e
	original := customerOriginal(forwardedCustomer, "original-1@prospect.example")

	env.syncSent(t, map[string]bool{"fwd-1@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-1@myco.example", original))

	got := forwardOf(t, e, "fwd-1@myco.example")
	// The note above the attached original is the rep's own words.
	if got.completeness != string(capturemod.BodyComplete) {
		t.Errorf("body_completeness = %q, want %q: the body is the rep's note", got.completeness, capturemod.BodyComplete)
	}
	assertNoExchange(t, e, got.id)
	if author := authorOf(t, e, got.id); got.counterparty != captureOwner || author != e.Rep1 {
		t.Errorf("counterparty %q, author %s: want the rep's own address and the rep, never the original's sender",
			got.counterparty, author)
	}
	if got.contactOwner != e.Rep1 || got.contact != "Bea Kunde" {
		t.Errorf("the new contact %q is owned by %s, want Bea Kunde owned by the rep who forwarded it (%s)",
			got.contact, got.contactOwner, e.Rep1)
	}
	// A forward is no answer to the original, so it starts its own thread.
	if got.thread != "fwd-1@myco.example" {
		t.Errorf("thread_key = %q, want the forward's own: joining the original's thread reads as a reply", got.thread)
	}
	if n := adminCount(t, e, `SELECT count(*) FROM activity_participant WHERE address = $1`, forwardedCustomer); n != 0 {
		t.Errorf("%d participant rows name the original's sender, want none: the original is not a party", n)
	}
	if n := adminCount(t, e, `
		SELECT count(*) FROM attachment at JOIN activity a ON a.id = at.activity_id
		 WHERE a.source_id = 'fwd-1@myco.example' AND NOT at.bytes_withheld`); n != 2 {
		t.Errorf("%d files kept with their bytes, want the offer and the original", n)
	}

	// The same original forwarded again is the same activity.
	env.syncSent(t, map[string]bool{"fwd-2@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-2@myco.example", original))
	if n := adminCount(t, e, `
		SELECT count(*) FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
		  JOIN contact_email ce ON ce.contact_id = l.contact_id AND ce.email = $1
		 WHERE a.kind = 'email' AND a.archived_at IS NULL`, forwardedCustomer); n != 1 {
		t.Errorf("%d activities filed under the customer after two forwards of one original, want 1", n)
	}
}

func TestForwardsFromTwoRepsFileUnderOneContact(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	colleague := secondSentMailbox(t, e, e.Rep3)
	original := customerOriginal(forwardedCustomer, "original-2@prospect.example")

	env.syncSent(t, map[string]bool{"fwd-a@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-a@myco.example", original))
	colleague(t, map[string]bool{"fwd-b@myco.example": true}, nil,
		forwardTo(secondSeatAddress, secondSeatAddress, "fwd-b@myco.example", original))

	first, second := forwardOf(t, e, "fwd-a@myco.example"), forwardOf(t, e, "fwd-b@myco.example")
	if a, b := importRowsOf(t, e, first.id), importRowsOf(t, e, second.id); len(a) != 1 || a[0] != e.Rep1 || len(b) != 1 || b[0] != e.Rep3 {
		t.Errorf("forwards submitted by %v and %v, want each rep their own (%s, %s)", a, b, e.Rep1, e.Rep3)
	}
	if second.contactOwner != e.Rep1 {
		t.Errorf("the second forward moved the contact to %s, want it left with %s", second.contactOwner, e.Rep1)
	}
	if n := adminCount(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, forwardedCustomer); n != 1 {
		t.Errorf("%d contacts hold the customer's address, want 1", n)
	}
}

// A forward sent from the rep's private alias to their connected mailbox is
// filed. One sent to that alias is not: it is where private mail goes.
func TestOnlyAForwardToTheConnectedMailboxIsFiled(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	env.syncSent(t, map[string]bool{"fwd-from-alias@myco.example": true, "fwd-to-alias@myco.example": true},
		forwardTo(ownerAlias, captureOwner, "fwd-from-alias@myco.example",
			customerOriginal(forwardedCustomer, "original-3@prospect.example")),
		forwardTo(captureOwner, ownerAlias, "fwd-to-alias@myco.example",
			customerOriginal("doctor@clinic.example", "original-3b@clinic.example")))

	forwardOf(t, e, "fwd-from-alias@myco.example")
	if n := adminCount(t, e, `SELECT count(*) FROM contact_email WHERE email = ANY($1)`,
		[]string{ownerAlias, "doctor@clinic.example"}); n != 0 {
		t.Errorf("%d contacts hold the rep's alias or the private sender, want none", n)
	}
	if n := adminCount(t, e, `SELECT count(*) FROM activity WHERE source_id = 'fwd-to-alias@myco.example'`); n != 0 {
		t.Errorf("the forward to the private alias was captured, want it skipped")
	}
}

// A marked inline forward is filed too, and its body, the quoted original, is
// marked as not the rep's own words.
func TestAnInlineForwardIsFiledAsTheOriginalsText(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	env.syncSent(t, map[string]bool{"fwd-inline@myco.example": true}, []byte(strings.Join([]string{
		"From: " + captureOwner, "To: " + captureOwner, "Subject: FW: Angebot",
		"Date: Thu, 04 Jun 2026 08:00:00 +0000", "Message-ID: <fwd-inline@myco.example>",
		"Content-Type: text/plain; charset=utf-8", "",
		"________________________________", "From: Bea Kunde <" + forwardedCustomer + ">",
		"Sent: Wednesday, June 3, 2026 9:00 AM", "To: " + captureOwner, "Subject: Angebot", "",
		"please send the offer", "",
	}, "\r\n")))

	if got := forwardOf(t, e, "fwd-inline@myco.example"); got.completeness != string(capturemod.BodyForwarded) {
		t.Errorf("body_completeness = %q, want %q: the body is the customer's text", got.completeness, capturemod.BodyForwarded)
	}
}

// The original's sender is refused when they are a colleague: a seat's login
// outside our domains, or anybody on our domains.
func TestAForwardOfAColleaguesMailIsNotFiled(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e

	env.syncSent(t, map[string]bool{"fwd-seat@myco.example": true, "fwd-domain@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-seat@myco.example",
			customerOriginal(colleagueLogin, "original-c1@search.test")),
		forwardTo(captureOwner, captureOwner, "fwd-domain@myco.example",
			customerOriginal("nobody@myco.example", "original-c2@myco.example")))

	for _, id := range []string{"fwd-seat@myco.example", "fwd-domain@myco.example"} {
		if n := adminCount(t, e, `SELECT count(*) FROM activity WHERE source_id = $1`, id); n != 0 {
			t.Errorf("%s was captured, want a colleague's mail skipped", id)
		}
		if n := adminCount(t, e, `
			SELECT count(*) FROM capture_trace WHERE source_id = $1 AND reason = 'own_addresses_only'`, id); n != 1 {
			t.Errorf("%s left %d own_addresses_only traces, want 1: it was no forward of a customer's mail", id, n)
		}
	}
}

// A forward is never more visible than its original. The original's thread
// was judged the rep's private life, so the forward creates nobody and stays
// held, though the mailbox shares its mail.
func TestAForwardKeepsTheOriginalThreadsPrivateVerdict(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	shareMailbox(t, env)
	seedPersonalThread(t, e, e.Rep1, "original-p@prospect.example", []string{forwardedCustomer})

	env.syncSent(t, map[string]bool{"fwd-private@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-private@myco.example",
			customerOriginal(forwardedCustomer, "original-p@prospect.example")))

	if n := adminCount(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, forwardedCustomer); n != 0 {
		t.Errorf("%d contacts for the sender of a private thread, want none", n)
	}
	if n := adminCount(t, e, `
		SELECT count(*) FROM activity WHERE source_id = 'fwd-private@myco.example' AND audience = 'workspace'`); n != 0 {
		t.Errorf("the forward of a private thread opened to the workspace, want it held")
	}
}

// assertNoExchange checks that a forward counts as nobody writing to the
// customer. It waits on no answer and reads as nobody's move. It leaves the
// customer's last exchange empty.
func assertNoExchange(t *testing.T, e *integration.SearchEnv, forward ids.UUID) {
	t.Helper()
	reader := humanWithScopes(e, e.Rep1, nil)
	later := captureNow.AddDate(0, 0, 7)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return later })
	waiting, _, err := store.AwaitingReplies(reader, later)
	if err != nil {
		t.Fatalf("reading the follow-up lane: %v", err)
	}
	for _, row := range waiting {
		if row.ActivityID == forward {
			t.Errorf("the forward waits on the customer's answer in the follow-up lane")
		}
	}
	var move crmcontracts.EmailSummaryMove
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		moves, err := activities.MailMovesFor(reader, tx, []ids.UUID{forward}, later)
		move = moves[forward]
		return err
	}); err != nil {
		t.Fatalf("reading the forward's move: %v", err)
	}
	if move != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("the forward's move = %q, want none", move)
	}
	var contact ids.UUID
	if err := adminRow(e, `
		SELECT l.contact_id FROM activity_link l WHERE l.activity_id = $1 AND l.entity_type = 'contact'`,
		[]any{forward}, &contact); err != nil {
		t.Fatalf("reading the forward's contact: %v", err)
	}
	latest, err := store.LatestInbound(reader, "contact", contact)
	if err != nil {
		t.Fatalf("reading the customer's last exchange: %v", err)
	}
	if latest.LastExchange != nil {
		t.Errorf("the customer's last exchange is %v, want none: nobody wrote to them", latest.LastExchange.Activity.Id)
	}
}

// Mail between the rep's own addresses that carries no customer's mail is
// still skipped, and the trace says why. So is a forward the provider never
// filed as sent: its From could be anybody's.
func TestOwnMailWithNoCustomerForwardIsSkippedWithItsReason(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	env.syncSent(t, map[string]bool{"note@myco.example": true, "own-original@myco.example": true},
		email(captureOwner, "", ownerAlias, "note@myco.example", ""),
		forwardTo(captureOwner, captureOwner, "own-original@myco.example",
			customerOriginal(ownerAlias, "original-4@private.example")))
	env.sync(t, forwardTo(captureOwner, captureOwner, "unfiled@myco.example",
		customerOriginal(forwardedCustomer, "original-5@prospect.example")))

	for _, id := range []string{"note@myco.example", "own-original@myco.example", "unfiled@myco.example"} {
		if n := adminCount(t, e, `SELECT count(*) FROM activity WHERE source_id = $1`, id); n != 0 {
			t.Errorf("%s was captured, want it skipped", id)
		}
		if n := adminCount(t, e, `
			SELECT count(*) FROM capture_trace WHERE source_id = $1 AND reason = 'own_addresses_only'`, id); n != 1 {
			t.Errorf("%s left %d own_addresses_only traces, want 1", id, n)
		}
	}
	if n := adminCount(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, forwardedCustomer); n != 0 {
		t.Errorf("the unfiled forward created %d contacts, want none", n)
	}
}

// The project comes only from the ladder. A customer with two open deals on
// two projects names neither. The forward is filed under no project and no
// deal, and the rep who forwarded it is asked.
func TestAForwardWhoseProjectIsAmbiguousAsksTheRep(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, forwardedCustomer, w.customer)
	w.dealOnProject(t, "Rollout", w.project(t, "Rollout").ID)
	w.dealOnProject(t, "Reporting", w.project(t, "Reporting").ID)

	env.syncSent(t, map[string]bool{"fwd-deal@myco.example": true},
		forwardTo(captureOwner, captureOwner, "fwd-deal@myco.example",
			customerOriginal(forwardedCustomer, "original-6@prospect.example")))

	wantDeals(t, e, "fwd-deal@myco.example")
	if n := adminCount(t, e, `
		SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		 WHERE a.source_id = 'fwd-deal@myco.example' AND l.entity_type = 'project'`); n != 0 {
		t.Errorf("the forward was filed under %d projects, want none", n)
	}
	n, holders := whichDealQuestions(t, e)
	if n != 1 || holders[0] != e.Rep1 {
		t.Fatalf("deal questions held by %v, want one for the rep who forwarded (%s)", holders, e.Rep1)
	}
}

// A file withheld because nobody here was addressed says so. It is not the
// private-message reason, which a reader would take as a judgement on them.
func TestAFileOfMailNobodyHereWasAddressedOnSaysWhy(t *testing.T) {
	t.Parallel()
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e := env.e
	env.sync(t, []byte(strings.Join([]string{
		"From: " + forwardedCustomer, "To: someone@elsewhere.example", "Cc: " + captureOwner,
		"Subject: Angebot", "Date: Thu, 04 Jun 2026 08:00:00 +0000", "Message-ID: <copied@prospect.example>",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="F"`, "",
		"--F", "Content-Type: text/plain; charset=utf-8", "", "the offer",
		"--F", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="offer.pdf"`, "", "%PDF-1.4 offer",
		"--F--", "",
	}, "\r\n")))

	var mail ids.UUID
	if err := adminRow(e, `SELECT id FROM activity WHERE source_id = 'copied@prospect.example'`, nil, &mail); err != nil {
		t.Fatalf("reading the copied mail: %v", err)
	}
	reader := humanWithScopes(e, e.Rep1, nil)
	got, err := activities.NewStore(e.DB()).GetEmailPresentation(reader, ids.From[ids.ActivityKind](mail), nil)
	if err != nil {
		t.Fatalf("reading the mail: %v", err)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("%d files, want the one the mail carried", len(got.Attachments))
	}
	file := got.Attachments[0]
	if file.BytesWithheld == nil || !*file.BytesWithheld || file.WithheldReason == nil ||
		*file.WithheldReason != crmcontracts.EmailAttachmentSummaryWithheldReasonNotAddressed {
		t.Errorf("withheld %v for %v, want withheld because nobody here was addressed", file.BytesWithheld, file.WithheldReason)
	}
}

// A seat with two mailboxes forwards from one to the other. Only the copy
// the receiving mailbox captured was sent to the mailbox that captured it.
func TestAForwardIsFiledOnlyByTheMailboxItWasSentTo(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	second := secondSentMailbox(t, e, e.Rep1)
	forward := forwardTo(captureOwner, secondSeatAddress, "fwd-between@myco.example",
		customerOriginal(forwardedCustomer, "original-m@prospect.example"))

	env.syncSent(t, map[string]bool{"fwd-between@myco.example": true}, forward)
	if n := adminCount(t, e, `SELECT count(*) FROM activity WHERE source_id = 'fwd-between@myco.example'`); n != 0 {
		t.Fatalf("the sending mailbox filed the forward, want it skipped: it was sent to the other mailbox")
	}
	second(t, map[string]bool{"fwd-between@myco.example": true}, nil, forward)
	forwardOf(t, e, "fwd-between@myco.example")
}
