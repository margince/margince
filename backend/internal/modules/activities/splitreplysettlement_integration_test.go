// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Our reply on a new topic sits in a conversation of its own, and its delivery
// names the request it answers. The request reads as answered, is offered for
// settlement, and a settled verdict closes its reminder.
func TestASplitReplySettlesTheRequestItAnswers(t *testing.T) {
	t.Parallel()
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Answered ask", "commitment", OwedVerdictAsksUs)
	replied := requestInstant.Add(time.Hour)
	splitReplyTo(t, e, request, replied)
	mintReminder(t, e, requestInstant.Add(2*time.Hour))

	e.stampSettled(t)
	if stamp, _ := e.stampOf(t, request); stamp == nil || !stamp.Equal(replied) {
		t.Errorf("the request's settled stamp is %v, want the reply's time %v", stamp, replied)
	}
	var answered bool
	if err := e.owner.QueryRow(context.Background(), `SELECT `+storekit.AnsweredSQL("a", "$1::timestamptz")+
		` FROM activity a WHERE a.id = $2`, requestInstant.Add(48*time.Hour), request).Scan(&answered); err != nil {
		t.Fatal(err)
	}
	if !answered {
		t.Error("the request does not read as answered by our reply on a new topic")
	}

	summaries, err := store.EmailSummariesByID(e.as(), []ids.UUID{request})
	if err != nil {
		t.Fatal(err)
	}
	if summaries[request].Move == crmcontracts.EmailSummaryMoveNeedsReply {
		t.Error("the request still reads as needing a reply after our reply on a new topic")
	}
	candidate, offered := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]
	if !offered || candidate.TaskID == nil {
		t.Fatalf("offered %v with reminder %v, want the request offered with its reminder", offered, candidate.TaskID)
	}
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, done, _ := taskFor(t, e, request); !done {
		t.Error("a settled verdict left the reminder open")
	}
}

// splitReplyTo writes our attested reply under a key of its own. Its sent
// delivery names the request it answers, as the send path writes them.
func splitReplyTo(t *testing.T, e *loadEnv, request ids.UUID, at time.Time) {
	t.Helper()
	subject, body, direction := "Invoice 4711 overdue", "Attached.", "outbound"
	key := ids.NewV7().String() + "@crm.example.test"
	reply, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: key, Source: "test", OccurredAt: &at,
		CounterpartyEmail: requestCounterparty, CounterpartyOutboundAttested: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `
		INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		                            consent_purpose, references_chain, status, answers_activity_id)
		VALUES ($1, $2, $3, 'gmail', $4, '[]'::jsonb, '[]'::jsonb, $5, $6, 'transactional', '[]'::jsonb, 'sent', $7)`,
		ids.NewV7(), ids.UUID(reply.Id), e.rep, key, subject, body, request)
}
