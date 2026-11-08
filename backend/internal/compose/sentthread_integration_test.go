// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReplyJoinsSentMailBeforeMailboxEcho(t *testing.T) {
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	root := "opening@counterparty.test"
	captureThreadMail(t, e, e.Rep1, threadMail(address, root, "Tue, 01 Sep 2026 10:00:00 +0000", ""))
	var anchor ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT activity_id FROM activity_identity WHERE identity_kind = 'mail' AND identity_key = $1`, root).Scan(&anchor)
	}); err != nil {
		t.Fatal(err)
	}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: &recordingStager{}})
	sent, err := adapter.SendEmail(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), anchor, agents.SendEmailArgs{To: []string{"pat@counterparty.example"}, Subject: "Re: Angebot", Body: "I will send the draft.", ConsentPurpose: "transactional"})
	if err != nil {
		t.Fatal(err)
	}
	var messageID, sentKey string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT source_id, thread_key FROM activity WHERE id = $1`, sent.ActivityID).Scan(&messageID, &sentKey)
	}); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, sent.ActivityID); count != 0 {
		t.Fatal("sent message unexpectedly has a captured copy")
	}
	reply := "reply-before-echo@counterparty.test"
	captureThreadMail(t, e, e.Rep1, threadMail(address, reply, "Thu, 01 Oct 2026 10:00:00 +0000", messageID, messageID))
	if got := threadKeyOfMessage(t, e, reply); got != sentKey {
		t.Fatalf("reply thread %q, want %q", got, sentKey)
	}
	otherReply := "other-seat@counterparty.test"
	captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), otherReply, "Thu, 01 Oct 2026 11:00:00 +0000", messageID, messageID))
	if got := threadKeyOfMessage(t, e, otherReply); got == sentKey {
		t.Fatal("another seat joined a sent message they do not hold")
	}
	echo := strings.Join([]string{
		"From: " + address,
		"To: pat@counterparty.example",
		"Subject: Re: Angebot",
		"Date: Sun, 18 Oct 2026 10:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"References: <" + root + ">",
		"Content-Type: text/plain; charset=utf-8", "", "I will send the draft.", "",
	}, "\r\n")
	captureThreadMail(t, e, e.Rep1, []byte(echo))
	captureThreadMail(t, e, e.Rep1, []byte(echo))
	if count := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'email' AND source_id = $1`, messageID); count != 1 {
		t.Fatalf("sent echo left %d messages", count)
	}
	if got := threadKeyOfMessage(t, e, reply); got != sentKey {
		t.Fatalf("sent echo changed reply thread to %q", got)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, sent.ActivityID); count != 1 {
		t.Fatalf("sent echo left %d import rows", count)
	}
}

func TestJoinedSentMailNeedsSemanticSettlement(t *testing.T) {
	for _, tc := range []struct {
		name, body, verdict, remaining string
		owed                           bool
	}{
		{"delivery", "Here is the pricing: ten seats cost EUR 100 per month.", activities.RequestSettled, "", false},
		{"acknowledgement", "Thanks, I will check and send the pricing.", activities.RequestStillOwed, "send the pricing", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			anchor := o.capture(t, owedMail{from: "pat@customer.example", to: o.seat, subject: "Please send pricing", messageID: "pricing-request@customer.example", at: o.now.Add(-3 * time.Hour)})

			adapter := newCommsAdapter(o.e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: &recordingStager{}})
			sender := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.SchedulerPerms)
			sent, err := adapter.SendEmail(sender, anchor, agents.SendEmailArgs{To: []string{"pat@customer.example"}, Subject: "Re: Please send pricing", Body: "Let me know which pricing you need.", ConsentPurpose: "transactional"})
			if err != nil {
				t.Fatal(err)
			}
			at := o.now.Add(-2 * time.Hour)
			if _, err := o.e.Activities.UpdateActivity(sender, ids.From[ids.ActivityKind](sent.ActivityID), activities.UpdateActivityInput{OccurredAt: &at}); err != nil {
				t.Fatal(err)
			}
			messageID := o.e.WsScalar(t, `SELECT source_id FROM activity WHERE id=$1`, sent.ActivityID)
			incoming := o.capture(t, owedMail{from: "pat@customer.example", to: o.seat, subject: "Re: Please send pricing", messageID: "reply-before-copy@customer.example", inReplyTo: messageID, at: o.now.Add(-time.Hour), body: "Could you send the pricing for ten seats?"})
			if o.threadKey(t, incoming) != o.threadKey(t, anchor) {
				t.Fatal("reply did not join the sent message before its mailbox copy")
			}
			if count := o.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1`, sent.ActivityID); count != 0 {
				t.Fatal("fixture already has a sent mailbox copy")
			}
			request := incoming
			if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs, "test", o.now); err != nil || !applied {
				t.Fatalf("request verdict: %v, %v", applied, err)
			}
			fulfillment, err := adapter.SendEmail(sender, anchor, agents.SendEmailArgs{To: []string{"pat@customer.example"}, Subject: "Re: Please send pricing", Body: tc.body, ConsentPurpose: "transactional"})
			if err != nil {
				t.Fatal(err)
			}
			fulfillmentAt := o.now.Add(-30 * time.Minute)
			if _, err := o.e.Activities.UpdateActivity(sender, ids.From[ids.ActivityKind](fulfillment.ActivityID), activities.UpdateActivityInput{OccurredAt: &fulfillmentAt}); err != nil {
				t.Fatal(err)
			}
			reply, err := json.Marshal(settlePayload{Results: []settleResult{{ID: request.String(), Verdict: tc.verdict, Remaining: tc.remaining, Confidence: 0.95}}})
			if err != nil {
				t.Fatal(err)
			}
			brain := ai.NewFakeClient().Script(string(reply))
			settler := NewRequestSettler(o.e.Pool, brain, func() time.Time { return o.now }, slog.Default())
			for range 2 {
				if err := settler.RunWorkspace(o.judge(), 1); err != nil {
					t.Fatal(err)
				}
			}
			calls := brain.Calls()
			if len(calls) != 1 || !strings.Contains(string(calls[0].Payload), tc.body) {
				t.Fatal("settlement did not read the joined evidence exactly once")
			}
			if got := o.e.WsCount(t, `SELECT count(*) FROM activity_request_settlement WHERE request_activity_id=$1`, request); got != 1 {
				t.Fatalf("settlement rows = %d", got)
			}
			if got := o.e.WsScalar(t, `SELECT verdict FROM activity_request_settlement WHERE request_activity_id=$1`, request); got != tc.verdict {
				t.Fatalf("settlement = %q, want %q", got, tc.verdict)
			}
			waiting, err := o.e.Activities.WaitingReplies(o.reader(), o.now)
			if err != nil {
				t.Fatal(err)
			}
			owed := false
			for _, row := range waiting {
				if row.ActivityID == request {
					owed = true
				}
			}
			if owed != tc.owed {
				t.Fatalf("waiting = %v, want %v", owed, tc.owed)
			}
		})
	}
}

func TestSentThreadJoinKeepsActorAndContentBoundaries(t *testing.T) {
	for _, name := range []string{"agent send", "selected sender", "selected other member", "full references"} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			address := seatAddress(t, e, e.Rep1)
			root := "opening@counterparty.test"
			captureThreadMail(t, e, e.Rep1, threadMail(address, root, "Tue, 01 Sep 2026 10:00:00 +0000", ""))
			var anchor ids.UUID
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(e.Admin(), `SELECT activity_id FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, root).Scan(&anchor)
			}); err != nil {
				t.Fatal(err)
			}
			sender := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
			if name == "agent send" {
				actor, ok := principal.Actor(sender)
				if !ok {
					t.Fatal("sender has no principal")
				}
				actor.Type, actor.ID, actor.OnBehalfOf = principal.PrincipalAgent, "agent:test-mail", e.Rep1
				sender = principal.WithActor(sender, actor)
			}
			adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: &recordingStager{}})
			sent, err := adapter.SendEmail(sender, anchor, agents.SendEmailArgs{To: []string{"pat@counterparty.example"}, Subject: "Re: Angebot", Body: "I will send the draft.", ConsentPurpose: "transactional"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(name, "selected") {
				member := e.Rep1
				if name == "selected other member" {
					member = e.Rep2
				}
				owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
				if _, err := e.Activities.SetAudience(owner, ids.From[ids.ActivityKind](sent.ActivityID), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: member}}}); err != nil {
					t.Fatal(err)
				}
			}
			denied := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{})
			if err := database.WithWorkspaceTx(denied, e.Pool, func(tx pgx.Tx) error {
				found, err := activities.SentFromHereBySeatTx(denied, tx, e.Rep1, []ids.ActivityID{ids.From[ids.ActivityKind](sent.ActivityID)})
				if len(found) != 0 {
					t.Fatal("sent-mail proof ignored the read grant")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			key := e.WsScalar(t, `SELECT thread_key FROM activity WHERE id=$1`, sent.ActivityID)
			messageID := e.WsScalar(t, `SELECT source_id FROM activity WHERE id=$1`, sent.ActivityID)
			refs, replyTo := []string{messageID}, messageID
			if name == "full references" {
				refs = []string{messageID}
				for range 30 {
					refs = append(refs, ids.NewV7().String()+"@unknown.example")
				}
				replyTo = refs[len(refs)-1]
			}
			reply := "boundary-reply@counterparty.test"
			captureThreadMail(t, e, e.Rep1, threadMail(address, reply, "Thu, 01 Oct 2026 10:00:00 +0000", replyTo, refs...))
			joined := threadKeyOfMessage(t, e, reply) == key
			want := name != "agent send"
			if joined != want {
				t.Fatalf("joined=%v, want %v", joined, want)
			}
			if name == "selected other member" {
				other := "other-member-reply@counterparty.test"
				captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), other, "Thu, 01 Oct 2026 11:00:00 +0000", messageID))
				if threadKeyOfMessage(t, e, other) == key {
					t.Fatal("content access was mistaken for ownership of the sent mailbox")
				}
			}
			if strings.HasPrefix(name, "selected") && e.WsScalar(t, `SELECT audience FROM activity WHERE id=$1`, sent.ActivityID) != "selected" {
				t.Fatal("joining widened the sent source audience")
			}
		})
	}
}
