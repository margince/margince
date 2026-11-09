// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A reply logged by hand, with no thread and no attested sending, answers
// nothing by the answer rule. The mail it replies to stays owed, and every
// surface says the move is ours, the last exchange included. A reply that
// names the mail it answers joins its thread instead.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// logMail files one email by hand on a contact, as an older log did: a link, a
// direction and a subject. It has no thread and no addresses.
func (o *owedEnv) logMail(t *testing.T, contact ids.UUID, direction, subject string, at time.Time) ids.UUID {
	t.Helper()
	logged, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindEmail), Direction: &direction, Subject: &subject,
		OccurredAt: &at, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the %s mail: %v", direction, err)
	}
	return ids.UUID(logged.Id)
}

func TestAHandLoggedReplyLeavesTheMoveWithUsWhereItLeavesTheMailOwed(t *testing.T) {
	o := setupOwed(t)
	company := ids.From[ids.CompanyKind](o.e.SeedCompany(t, "Logged By Hand GmbH", nil))
	contact := o.contact(t, "Pat Logged", "pat@logged.example")
	contactID, primary := ids.From[ids.ContactKind](contact), true
	if _, err := o.e.Contacts.CreateRelationship(o.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &company, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	// A fortnight back, so the account strip reads the exchange as settled
	// rather than as fresh activity.
	fortnight := o.now.Add(-14 * 24 * time.Hour)
	theirs := o.logMail(t, contact, "inbound", "Can we talk pricing?", fortnight)
	o.logMail(t, contact, "outbound", "Re: Can we talk pricing?", fortnight.Add(time.Hour))

	var badge crmcontracts.EmailSummaryMove
	if err := database.WithWorkspaceTx(o.reader(), o.e.Pool, func(tx pgx.Tx) error {
		moves, err := activities.MailMovesFor(o.reader(), tx, []ids.UUID{theirs}, o.now)
		badge = moves[theirs]
		return err
	}); err != nil {
		t.Fatalf("reading the badge: %v", err)
	}
	if badge != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("their mail reads %q, want needs_reply: an unthreaded, unattested reply is no answer", badge)
	}

	latest, err := o.e.Activities.LatestInbound(o.reader(), "contact", contact)
	if err != nil {
		t.Fatalf("reading the contact's latest inbound: %v", err)
	}
	if latest.LastExchange == nil {
		t.Fatal("the contact has no last exchange")
	}
	if got := latest.LastExchange.WhoseMove; got != crmcontracts.LastExchangeWhoseMoveUs {
		t.Fatalf("the last exchange reads whose_move %q, want us, as the badge says", got)
	}

	state, advised, engagement := o.whoseMove(t, company)
	if state != "waiting_on_us" || advised {
		t.Errorf("the account strip reads %q with no-reply advice %v, want waiting_on_us and no advice", state, advised)
	}
	if engagement != crmcontracts.ContactEngagementWaiting {
		t.Errorf("the contact reads %q, want waiting", engagement)
	}
}

// logThroughCreate files one email through the create request mapping, the
// door POST /v1/activities and the log_activity verb share. The request is
// decoded from the JSON a caller sends.
func (o *owedEnv) logThroughCreate(ctx context.Context, t *testing.T, contact ids.UUID, direction, subject string, at time.Time, replyTo *ids.UUID) (ids.UUID, error) {
	t.Helper()
	body := map[string]any{
		"kind": "email", "direction": direction, "subject": subject, "source": "manual",
		"occurred_at": at.Format(time.RFC3339Nano),
		"links":       []map[string]any{{"entity_type": "contact", "entity_id": contact.String()}},
	}
	if replyTo != nil {
		body["in_reply_to_activity_id"] = replyTo.String()
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var req crmcontracts.CreateActivityRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decoding the create request: %v", err)
	}
	in, err := activities.LogActivityInputFrom(req)
	if err != nil {
		return ids.Nil, err
	}
	logged, _, err := o.e.Activities.LogActivity(ctx, in)
	return ids.UUID(logged.Id), err
}

func (o *owedEnv) threadOf(t *testing.T, activity ids.UUID) string {
	t.Helper()
	return o.e.WsScalar(t, `SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, activity)
}

func (o *owedEnv) badgeOf(t *testing.T, activity ids.UUID) crmcontracts.EmailSummaryMove {
	t.Helper()
	var badge crmcontracts.EmailSummaryMove
	if err := database.WithWorkspaceTx(o.reader(), o.e.Pool, func(tx pgx.Tx) error {
		moves, err := activities.MailMovesFor(o.reader(), tx, []ids.UUID{activity}, o.now)
		badge = moves[activity]
		return err
	}); err != nil {
		t.Fatalf("reading the badge: %v", err)
	}
	return badge
}

// A reply logged against the mail it answers joins that mail's thread, so the
// thread rule reads it as the answer.
func TestALoggedReplyOnTheMailsThreadAnswersIt(t *testing.T) {
	o := setupOwed(t)
	contact := o.contact(t, "Pat Threaded", "pat@threaded.example")
	at := o.now.Add(-3 * time.Hour)
	theirs, err := o.logThroughCreate(o.reader(), t, contact, "inbound", "Can we talk pricing?", at, nil)
	if err != nil {
		t.Fatalf("logging their mail: %v", err)
	}
	if thread := o.threadOf(t, theirs); !strings.HasPrefix(thread, "logged:") {
		t.Fatalf("a hand-logged mail is on thread %q, want one of its own", thread)
	}
	if got := o.badgeOf(t, theirs); got != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("before the reply their mail reads %q, want needs_reply", got)
	}
	ours, err := o.logThroughCreate(o.reader(), t, contact, "outbound", "Re: Can we talk pricing?", at.Add(time.Hour), &theirs)
	if err != nil {
		t.Fatalf("logging the reply: %v", err)
	}
	if o.threadOf(t, ours) != o.threadOf(t, theirs) {
		t.Fatalf("the reply is on thread %q, their mail on %q", o.threadOf(t, ours), o.threadOf(t, theirs))
	}
	if got := o.badgeOf(t, theirs); got == crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("their mail still reads needs_reply after a reply on its thread")
	}
}

// A mail logged before hand-logged mail kept a thread has none to join. The
// reply is accepted on a thread of its own, the mail stays owed, and the last
// exchange says so.
func TestAReplyToAMailWithNoThreadIsAcceptedAndTheMoveStaysOurs(t *testing.T) {
	o := setupOwed(t)
	contact := o.contact(t, "Pat Old", "pat@old.example")
	theirs := o.logMail(t, contact, "inbound", "Can we talk pricing?", o.now.Add(-3*time.Hour))
	if thread := o.threadOf(t, theirs); thread != "" {
		t.Fatalf("the old mail is on thread %q, want none", thread)
	}
	ours, err := o.logThroughCreate(o.reader(), t, contact, "outbound", "Re: Can we talk pricing?", o.now.Add(-2*time.Hour), &theirs)
	if err != nil {
		t.Fatalf("a reply to an old mail was refused: %v", err)
	}
	if thread := o.threadOf(t, ours); !strings.HasPrefix(thread, "logged:") {
		t.Fatalf("the reply is on thread %q, want one of its own", thread)
	}
	if got := o.badgeOf(t, theirs); got != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("the old mail reads %q, want needs_reply", got)
	}
	latest, err := o.e.Activities.LatestInbound(o.reader(), "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	if latest.LastExchange == nil || latest.LastExchange.WhoseMove != crmcontracts.LastExchangeWhoseMoveUs {
		t.Fatal("the last exchange does not leave the move with us, while the badge does")
	}
}

// Each check on the mail a reply names refuses the reply, naming the field.
func TestAReplyIsRefusedForAMailItCannotJoin(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Refused", "pat@refused.example")
	sam := o.contact(t, "Sam Other", "sam@other.example")
	at := o.now.Add(-3 * time.Hour)
	theirs, err := o.logThroughCreate(o.reader(), t, pat, "inbound", "Can we talk pricing?", at, nil)
	if err != nil {
		t.Fatal(err)
	}
	note := "Called about pricing"
	noted, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: "note", Body: &note, OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: pat}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A mail kept to its participants, which the replying colleague is not one of.
	private, err := o.logThroughCreate(o.reader(), t, pat, "inbound", "Private terms", at, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET audience = 'participants' WHERE id = $1`, private)
		return err
	}); err != nil {
		t.Fatalf("keeping the mail to its participants: %v", err)
	}
	colleague := o.e.As(o.e.Rep2, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Create: true}, "contact": {Read: true}},
	})
	for _, c := range []struct {
		name    string
		ctx     context.Context
		contact ids.UUID
		replyTo ids.UUID
	}{
		{"a mail the caller cannot read", colleague, pat, private},
		{"a note", o.reader(), pat, ids.UUID(noted.Id)},
		{"another correspondent's mail", o.reader(), sam, theirs},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := o.logThroughCreate(c.ctx, t, c.contact, "outbound", "Re: pricing", at.Add(time.Hour), &c.replyTo)
			var refused *activities.ReplyFieldError
			if !errors.As(err, &refused) || refused.Field != "in_reply_to_activity_id" {
				t.Fatalf("logging a reply to %s = %v, want a refusal naming in_reply_to_activity_id", c.name, err)
			}
		})
	}
}
