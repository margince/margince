// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealscout_test

// The fixture every request test shares: captured buyer mail, read by the
// product's own signal pass, then scouted and opened as the job does.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const requestSummary = "They ask for a quote for 40 seats of the platform."

type requestEnv struct {
	*integration.Env
	now time.Time
}

func setupRequests(t *testing.T) *requestEnv {
	t.Helper()
	e := integration.Setup(t)
	integration.DealFixture(t, e)
	e.GrantRole(t, e.AdminUser, "admin")
	return &requestEnv{Env: e, now: clocktest.Now(t).Truncate(time.Microsecond)}
}

func (e *requestEnv) clock() time.Time { return e.now }

// mail is one inbound message a test captures.
type mail struct {
	// role is the buyer's header on it: "from", or "cc" for a copied contact.
	role string
	// authenticated says the sender's address was verified at capture.
	authenticated bool
	// reason is the audience reason that makes it private, or "" for shared.
	reason string
	// thread is the conversation it joins; empty starts its own.
	thread string
}

// sent is the usual request: from the buyer, authenticated, shared.
var sent = mail{role: "from", authenticated: true}

// capture writes one connector-captured inbound message from a buyer working at
// company, two days before the clock, imported from the admin's mailbox.
func (e *requestEnv) capture(t *testing.T, company ids.UUID, body string, m mail) (ids.UUID, ids.UUID) {
	t.Helper()
	owner := integration.OwnerConn(t)
	buyer := e.SeedContact(t, "Mira Buyer", &e.AdminUser)
	audience := "workspace"
	if m.reason != "" {
		audience = "participants"
	}
	message := ids.NewV7()
	thread := m.thread
	if thread == "" {
		thread = message.String()
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'manual', 'human:x')`, []any{buyer, company}},
		{`INSERT INTO activity (id, kind, direction, subject, body, thread_key, occurred_at, created_at, source,
			captured_by, audience, audience_reason, counterparty_email, inbound_sender_authenticated)
			VALUES ($1, 'email', 'inbound', 'Platform', $2, $7, $3, $3, 'gmail', 'connector:gmail', $4,
			nullif($5, ''), 'mira@acme.example', $6)`, []any{message, body, e.now.AddDate(0, 0, -2), audience, m.reason, m.authenticated, thread}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{message, buyer}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, $3)`, []any{message, buyer, m.role}},
		{`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, []any{message, e.AdminUser}},
	} {
		if _, err := owner.Exec(context.Background(), statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return message, buyer
}

// fixedReply answers every reading with one reply and counts the readings.
type fixedReply struct {
	reply string
	asked *int
}

func (f fixedReply) Complete(context.Context, model.Request) (model.Response, error) {
	*f.asked++
	return model.Response{Text: f.reply}, nil
}

// read runs one signal pass whose model reports events, and answers how many
// readings it was asked for.
func (e *requestEnv) read(t *testing.T, events ...map[string]any) int {
	t.Helper()
	reply, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatal(err)
	}
	asked := 0
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:signal-scan")
	if _, err := compose.NewSignalExtractor(e.Pool, fixedReply{reply: string(reply), asked: &asked}, e.clock, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("the signal pass: %v", err)
	}
	return asked
}

// opportunity is the model's report of a buyer's need in message, of kind.
func (e *requestEnv) opportunity(message, buyer ids.UUID, kind, budget string) map[string]any {
	return map[string]any{
		"kind": "new_opportunity", "message_id": message.String(), "summary": requestSummary,
		"quote": "Please send a quotation.", "due_date": "", "confidence": 0.9,
		"commercial_role": "buyer", "buyer_ref": "contact:" + buyer.String(),
		dealscout.RequestKindKey: kind, dealscout.BudgetQuoteKey: budget,
		dealscout.DecisionDateKey: e.decisionDay(),
	}
}

func (e *requestEnv) decisionDay() string { return e.now.AddDate(0, 1, 0).Format(time.DateOnly) }

// request captures a buyer's mail and has the signal pass file it as kind.
func (e *requestEnv) request(t *testing.T, company ids.UUID, kind, budget string, m mail) ids.UUID {
	t.Helper()
	message, buyer := e.capture(t, company, "We need a platform for 40 seats. Please send a quotation. "+budget, m)
	e.read(t, e.opportunity(message, buyer, kind, budget))
	return message
}

// openScouted runs the scout pass and then the opener, as the job does.
func (e *requestEnv) openScouted(t *testing.T) int {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:deal-scout")
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := compose.RunDealScout(ctx, tx, e.now)
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	opened, err := compose.OpenScoutedDeals(ctx, e.Pool, slog.New(slog.DiscardHandler), e.clock)
	if err != nil {
		t.Fatalf("opening the suggested deals: %v", err)
	}
	return opened
}

// openedDeal is the company's one opened deal as the reader in ctx reads it.
func (e *requestEnv) openedDeal(ctx context.Context, t *testing.T, company ids.UUID) crmcontracts.Deal {
	t.Helper()
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM deal WHERE company_id = $1 AND archived_at IS NULL`, company))
	if err != nil {
		t.Fatalf("no deal opened on the company: %v", err)
	}
	deal, err := e.Deals.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the opened deal: %v", err)
	}
	return deal
}

// openedCorrection is the deal_open correction on the company's deal.
func (e *requestEnv) openedCorrection(t *testing.T, company ids.UUID) deals.DealCorrection {
	t.Helper()
	auditID, err := ids.Parse(e.WsScalar(t, `SELECT c.audit_log_id::text FROM deal_correction c JOIN deal d ON d.id = c.deal_id
		 WHERE d.company_id = $1 AND c.correction = 'deal_open'`, company))
	if err != nil {
		t.Fatalf("no deal_open correction on the company: %v", err)
	}
	var correction deals.DealCorrection
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		correction, err = e.Deals.CorrectionForAudit(e.Admin(), tx, auditID)
		return err
	}); err != nil {
		t.Fatalf("reading the opened deal's correction: %v", err)
	}
	return correction
}

// switchOff turns an installation-wide switch off until the test ends.
func switchOff(t *testing.T, key string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ($1, 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, key); err != nil {
			t.Errorf("restoring %s: %v", key, err)
		}
	})
}

// shareKey is the switch that shares a routine-private request.
var shareKey = activities.SharePrivateQuoteRequests.Key()
