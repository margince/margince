// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The event pass reads a conversation only when every message on it was read
// in full. Each case captures mail through the real parser and sink and runs
// the real pass, with the model as the one replaced boundary.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// askedBrain records which messages each reading was shown, and answers with
// the events reply names for them.
type askedBrain struct {
	mu    sync.Mutex
	asked []string
	reply func(prompt string) string
}

func (b *askedBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	var prompt strings.Builder
	prompt.WriteString(req.System)
	for _, m := range req.Messages {
		prompt.WriteString(m.Content)
	}
	b.mu.Lock()
	b.asked = append(b.asked, prompt.String())
	b.mu.Unlock()
	if b.reply == nil {
		return model.Response{Text: `{"events":[]}`}, nil
	}
	return model.Response{Text: b.reply(prompt.String())}, nil
}

// timesShown is how many readings were shown the message.
func (b *askedBrain) timesShown(message ids.UUID) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, prompt := range b.asked {
		if strings.Contains(prompt, message.String()) {
			n++
		}
	}
	return n
}

// signalEnv is a shared mailbox whose customer works for one company, so a
// captured conversation reaches one account and the pass may read it.
func signalEnv(t *testing.T) *owedEnv {
	t.Helper()
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	company := o.e.SeedCompany(t, "Acme", &o.owner)
	contact := o.contact(t, "Pat Buyer", "pat@customer.example")
	o.e.WsExec(t, `INSERT INTO relationship (kind,contact_id,company_id,source,captured_by)
		VALUES ('employment',$1,$2,'manual','human:x')`, contact, company)
	return o
}

func (o *owedEnv) customerSays(t *testing.T, key, inReplyTo, body, encoding string, at time.Time) ids.UUID {
	t.Helper()
	return o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Agreement", messageID: key, inReplyTo: inReplyTo,
		body: body, encoding: encoding, authenticated: true, at: at,
	})
}

func (o *owedEnv) runEventPass(t *testing.T, brain *askedBrain) {
	t.Helper()
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), o.e.WS), signalScanActor)
	later := func() time.Time { return o.now.Add(72 * time.Hour) }
	if _, err := NewSignalExtractor(o.e.Pool, brain, later, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](o.e.WS)); err != nil {
		t.Fatalf("running the event pass: %v", err)
	}
}

// Conversations the pass may not read are refused before its cap. More of them
// than the cap holds cannot keep a readable conversation waiting for good.
func TestConversationsNotReadInFullDoNotFillTheEventPass(t *testing.T) {
	o := signalEnv(t)
	readable := o.customerSays(t, "readable@customer.example", "", "We are happy with the service.", "", o.now.Add(-10*time.Hour))
	for n := range extractThreadCap + 5 {
		o.customerSays(t, fmt.Sprintf("blocked-%d@customer.example", n), "",
			brokenAfter("We may change our plans."), "base64", o.now.Add(-time.Duration(n+1)*time.Minute))
	}
	brain := &askedBrain{}
	o.runEventPass(t, brain)
	if got := brain.timesShown(readable); got != 1 {
		t.Fatalf("the readable conversation was read %d times, want once behind %d blocked ones",
			got, extractThreadCap+5)
	}
	if got := len(brain.asked); got != 1 {
		t.Errorf("the pass made %d readings, want only the readable conversation", got)
	}
}

// A later message read only in part may withdraw an older one. The pass reads
// neither, so the older statement is not raised as the current state.
func TestALaterMessageReadOnlyInPartKeepsAnOlderEventFromBeingRaised(t *testing.T) {
	ended := "We are ending the agreement with your company."
	for _, tc := range []struct {
		name, later string
		raised      bool
	}{
		{"withdrawn in a mail read in part", brokenAfter("Please disregard that; we are continuing."), false},
		{"nothing after it", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := signalEnv(t)
			first := o.customerSays(t, "ending@customer.example", "", ended, "", o.now.Add(-3*time.Hour))
			if tc.later != "" {
				o.customerSays(t, "withdrawal@customer.example", "ending@customer.example", tc.later, "base64", o.now.Add(-2*time.Hour))
			}
			brain := &askedBrain{reply: func(string) string {
				reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
					"kind": signalKindContractEnded, "message_id": first.String(), "summary": "Acme ends the agreement.",
					"quote": ended, "lifecycle_scope": lifecycleScopeCompany, "confidence": 0.95,
				}}})
				if err != nil {
					t.Fatal(err)
				}
				return string(reply)
			}}
			o.runEventPass(t, brain)
			signals := o.e.WsCount(t, `SELECT count(*) FROM signal WHERE kind = 'contract_ended'`)
			if raised := signals > 0; raised != tc.raised {
				t.Fatalf("contract_ended raised = %v, want %v (readings: %d)", raised, tc.raised, len(brain.asked))
			}
		})
	}
}

// A conversation blocked by an unassessed message is read once the data task
// assesses it, and not again while nothing changes.
func TestAnAssessedConversationIsReadOnce(t *testing.T) {
	o := signalEnv(t)
	first := o.customerSays(t, "assess-1@customer.example", "", "Thanks for the call.", "", o.now.Add(-3*time.Hour))
	second := o.customerSays(t, "assess-2@customer.example", "assess-1@customer.example",
		"We would like to add ten seats.", "", o.now.Add(-2*time.Hour))
	o.asLegacy(t, second, "")

	brain := &askedBrain{}
	o.runEventPass(t, brain)
	if got := brain.timesShown(first); got != 0 {
		t.Fatalf("a conversation holding an unassessed message was read %d times", got)
	}
	o.runMailBodyTask(t)
	o.runEventPass(t, brain)
	if got := brain.timesShown(second); got != 1 {
		t.Fatalf("the assessed conversation was read %d times, want once", got)
	}
	o.runEventPass(t, brain)
	if got := brain.timesShown(second); got != 1 {
		t.Errorf("an unchanged conversation was read again: %d readings", got)
	}
}
