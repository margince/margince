// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealscout_test

// Which findings the signal pass keeps from private mail, through the pass
// itself with a fixed model reply.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// fixedReply answers every reading with one reply and counts the readings.
type fixedReply struct {
	reply string
	asked *int
}

func (f fixedReply) Complete(context.Context, model.Request) (model.Response, error) {
	*f.asked++
	return model.Response{Text: f.reply}, nil
}

// readThread captures a two-message thread, the second limited for reason, and
// runs one signal pass. Its model finds a need in the first message and an
// explicit request in the second. It answers how many readings were asked.
func readThread(t *testing.T, e *integration.Env, reason string) int {
	t.Helper()
	owner := integration.OwnerConn(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Mira Buyer", &e.Rep1)
	if _, err := owner.Exec(context.Background(), `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company); err != nil {
		t.Fatal(err)
	}
	now := clocktest.Now(t)
	at := now.Add(-48 * time.Hour)
	var messages [2]ids.UUID
	for i, audience := range []string{"workspace", "participants"} {
		messages[i] = ids.NewV7()
		if _, err := owner.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, body, thread_key, occurred_at, created_at,
			                      source, captured_by, audience, audience_reason, counterparty_email)
			VALUES ($1, 'email', 'inbound', 'Platform', 'We need a platform. Please send a quotation.', 'thread-request',
			        $2, $2, 'gmail', 'connector:gmail', $3, nullif($4, ''), 'mira@acme.example')`,
			messages[i], at.Add(time.Duration(i)*time.Minute), audience, map[bool]string{true: reason}[i == 1]); err != nil {
			t.Fatal(err)
		}
		integration.LinkActivity(t, owner, messages[i], "contact", contact)
	}
	event := func(message ids.UUID, kind string) map[string]any {
		return map[string]any{
			"kind": "new_opportunity", "message_id": message.String(), "summary": "They want a platform.",
			"quote": "Please send a quotation.", "due_date": "", "confidence": 0.9, "commercial_role": "unknown",
			dealscout.RequestKindKey: kind, dealscout.BudgetQuoteKey: "", dealscout.DecisionDateKey: "",
		}
	}
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{
		event(messages[0], dealscout.RequestKindNeed), event(messages[1], dealscout.RequestKindExplicit),
	}})
	if err != nil {
		t.Fatal(err)
	}
	asked := 0
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:signal-scan")
	if _, err := compose.NewSignalExtractor(e.Pool, fixedReply{reply: string(reply), asked: &asked}, func() time.Time { return now }, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("the signal pass: %v", err)
	}
	return asked
}

// Mail private only by a mailbox setting is read while the switch is on, and
// only its explicit request is filed, shared. Mail someone protected, or any
// private mail with the switch off, is never read.
func TestOnlyAnExplicitRequestIsKeptFromRoutinePrivateMail(t *testing.T) {
	e := integration.Setup(t)
	if asked := readThread(t, e, activities.ReasonPosture); asked == 0 {
		t.Fatal("the routine-private thread was not read")
	}
	kinds := e.WsScalar(t, `SELECT coalesce(string_agg(evidence->0->'fields'->>'request_kind' || ':' || visibility, ','), '')
		FROM signal WHERE kind = 'new_opportunity'`)
	if kinds != dealscout.RequestKindExplicit+":workspace" {
		t.Errorf("the pass filed %q, want only the explicit request, shared", kinds)
	}

	for name, reason := range map[string]string{"protected": activities.ReasonManual, "switched off": activities.ReasonPosture} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			if name == "switched off" {
				switchOff(t)
			}
			if asked := readThread(t, e, reason); asked != 0 {
				t.Errorf("the model was asked %d times about mail it may not read", asked)
			}
		})
	}
}
