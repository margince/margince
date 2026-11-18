// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ourReplyAt is the request's our_reply as the deal card's current-request
// read sees it at asOf. The bool says whether the request is current.
func ourReplyAt(t *testing.T, e *loadEnv, request ids.UUID, asOf time.Time) (*crmcontracts.RequestReply, bool) {
	t.Helper()
	kind := string(crmcontracts.ActivityKindEmail)
	rows, _, err := readingAt(storeKnowing(e), asOf).ListActivities(e.as(), ListActivitiesInput{
		RequestCurrentAsOf: &asOf, Kind: &kind, ReadableOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == request {
			if row.EmailSummary == nil {
				t.Fatal("the current request carries no email summary")
			}
			return row.EmailSummary.OurReply, true
		}
	}
	return nil, false
}

// asRepCompleting is the reminder's assignee with the grant the task write needs.
func asRepCompleting(t *testing.T, e *loadEnv) context.Context {
	t.Helper()
	ctx := e.as()
	rep, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	rep.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	return principal.WithActor(ctx, rep)
}

// The reported case: the buyer asked, the reminder was filed, and our reply
// came hours later. The request stays current, and the one rule says we
// replied, with the reminder the reader may mark done.
func TestOurLaterReplyMarksTheRequestReplied(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Please send the report", "commitment", OwedVerdictAsksUs)
	mintReminder(t, e, requestInstant.Add(time.Hour))
	replied := requestInstant.Add(5 * time.Hour)
	replyTo(t, e, request, "Thanks, I will check.", replied)

	reply, current := ourReplyAt(t, e, request, requestInstant.Add(72*time.Hour))
	if !current {
		t.Fatal("our reply settled the request by itself")
	}
	if reply == nil || !reply.RepliedAt.Equal(replied) {
		t.Fatalf("our reply = %+v, want one replied at %s", reply, replied)
	}
	reminder, _, _, _ := taskFor(t, e, request)
	if reply.ReminderId == nil || ids.UUID(*reply.ReminderId) != reminder || reply.ReminderVersion == nil {
		t.Fatal("our reply does not name the reminder its reader may complete")
	}
	if reply.StillAsked != nil {
		t.Fatal("a request no settlement judged quotes a remaining ask")
	}
}

func TestOurReplyQuotesWhatTheSettlementFoundStillAsked(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Report and slides", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here are the slides.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	asOf := requestInstant.Add(48 * time.Hour)
	if err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: repliedIDs(t, e, asOf)[request], Verdict: RequestStillOwed, Remaining: "send the report",
		Confidence: 0.9, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}

	reply, current := ourReplyAt(t, e, request, asOf)
	if !current || reply == nil {
		t.Fatal("a still_owed request lost its replied state")
	}
	if reply.StillAsked == nil || *reply.StillAsked != "send the report" {
		t.Fatalf("still asked = %v, want the quoted ask", reply.StillAsked)
	}
}

// Their newer message on the thread makes it our move again.
func TestTheirNewerMessageTakesTheReplyBack(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Pricing question", "commitment", OwedVerdictAsksUs)
	mintReminder(t, e, requestInstant.Add(time.Hour))
	replyTo(t, e, request, "Let me check.", requestInstant.Add(2*time.Hour))
	var key string
	if err := e.owner.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
		t.Fatal(err)
	}
	subject, body, direction, later := "Re: Pricing question", "Any news?", "inbound", requestInstant.Add(3*time.Hour)
	if _, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, ThreadKey: key,
		Source: "test", OccurredAt: &later, CounterpartyEmail: requestCounterparty,
	}); err != nil {
		t.Fatal(err)
	}

	if reply, current := ourReplyAt(t, e, request, requestInstant.Add(24*time.Hour)); !current || reply != nil {
		t.Fatalf("their newest message still reads as our reply (current %v, reply %+v)", current, reply)
	}
}

func TestARequestWithNoReplyHasNoReplyState(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Unanswered pricing", "commitment", OwedVerdictAsksUs)
	mintReminder(t, e, requestInstant.Add(time.Hour))

	if reply, current := ourReplyAt(t, e, request, requestInstant.Add(24*time.Hour)); !current || reply != nil {
		t.Fatalf("an unanswered request reads as replied (current %v, reply %+v)", current, reply)
	}
}

// The rule offers a reminder to complete, so a request no reminder covers
// stays the reply owed, however we answered.
func TestARequestWithNoReminderHasNoReplyState(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "No reminder filed", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "On it.", requestInstant.Add(time.Hour))

	if reply, current := ourReplyAt(t, e, request, requestInstant.Add(24*time.Hour)); !current || reply != nil {
		t.Fatalf("a request with no reminder reads as replied (current %v, reply %+v)", current, reply)
	}
}

// Done goes through the ordinary task write. The request then leaves the
// current set, so the card, its risk and its move stop naming it.
func TestCompletingTheNamedReminderSettlesTheRequest(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Contract draft", "commitment", OwedVerdictAsksUs)
	mintReminder(t, e, requestInstant.Add(time.Hour))
	replyTo(t, e, request, "Draft attached.", requestInstant.Add(2*time.Hour))
	asOf := requestInstant.Add(24 * time.Hour)
	reply, _ := ourReplyAt(t, e, request, asOf)
	if reply == nil || reply.ReminderId == nil {
		t.Fatal("no reminder to complete")
	}

	done := true
	if _, err := storeKnowing(e).UpdateActivity(asRepCompleting(t, e), ids.From[ids.ActivityKind](ids.UUID(*reply.ReminderId)),
		UpdateActivityInput{IsDone: &done, IfVersion: reply.ReminderVersion}); err != nil {
		t.Fatal(err)
	}
	if _, current := ourReplyAt(t, e, request, asOf); current {
		t.Fatal("the request stayed current after its reminder was marked done")
	}
}
