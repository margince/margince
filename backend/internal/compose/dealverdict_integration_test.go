// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince reads an open deal's evidence at night and sets its close date,
// amount and probability as its owner's agent. These tests drive the real
// pass over deals and notes written through the deals and activities stores;
// only the model is a stand-in.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// dealVerdictLane answers every deal with the same three changes, citing the
// note the test logged. It counts its calls.
type dealVerdictLane struct {
	cite  *string
	date  string
	calls int
}

func (l *dealVerdictLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: fmt.Sprintf(`{"actions":[
		{"kind":"set_close_date","date":%q,"amount":0,"currency":"","quoted_words":"","probability":0,
		 "cited_activity_ids":[%q],"reason":"They will sign then","confidence":0.9},
		{"kind":"set_amount","date":"","amount":45000,"currency":"EUR","quoted_words":"45,000 EUR a year",
		 "probability":0,"cited_activity_ids":[%q],"reason":"They named the price","confidence":0.9},
		{"kind":"set_probability","date":"","amount":0,"currency":"","quoted_words":"","probability":80,
		 "cited_activity_ids":[%q],"reason":"They said yes","confidence":0.9}]}`,
		l.date, *l.cite, *l.cite, *l.cite)}, nil
}

type dealVerdictEnv struct {
	*integration.Env
	deal ids.UUID
	note string
	lane *dealVerdictLane
	pass dealVerdictPass
	now  time.Time
}

func setupDealVerdict(t *testing.T) *dealVerdictEnv {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	pipeline, open, _ := integration.DealFixture(t, e)
	owner, amount, currency := ids.From[ids.UserKind](e.Rep1), int64(4_000_000), "EUR"
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Support rollout", PipelineID: pipeline, StageID: open, OwnerID: &owner,
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	v := &dealVerdictEnv{Env: e, deal: ids.UUID(created.Id)}
	// The note is dated after the deal was created, so no colleague's edit is
	// newer than the evidence until a test makes one.
	v.now = v.dbNow(t)
	v.note = v.logNote(t, "The board agreed: we sign for 45,000 EUR a year.")
	v.lane = &dealVerdictLane{cite: &v.note, date: v.now.AddDate(0, 2, 0).Format(time.DateOnly)}
	v.pass = newDealVerdictPass(e.Pool, v.lane, slog.New(slog.DiscardHandler))
	v.pass.now = func() time.Time { return v.now.Add(time.Minute) }
	return v
}

func (v *dealVerdictEnv) dbNow(t *testing.T) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, v.WsScalar(t, `SELECT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`))
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func (v *dealVerdictEnv) logNote(t *testing.T, body string) string {
	t.Helper()
	subject, at := "Call notes", v.now
	note, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: v.deal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return note.Id.String()
}

func (v *dealVerdictEnv) run(t *testing.T) {
	t.Helper()
	if err := v.pass.RunWorkspace(principal.SystemActing(v.Admin(), closeDateSweepActor)); err != nil {
		t.Fatal(err)
	}
}

func (v *dealVerdictEnv) owner(t *testing.T) context.Context {
	t.Helper()
	ctx, err := dealOwnerAuthority{db: InstallationDB(v.Pool), users: identity.NewService(v.Pool)}.asOwner(v.Admin(), v.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func (v *dealVerdictEnv) field(t *testing.T, column string) string {
	t.Helper()
	return v.WsScalar(t, `SELECT coalesce(`+column+`::text, '') FROM deal WHERE id = $1`, v.deal)
}

func (v *dealVerdictEnv) outcomes(t *testing.T) string {
	t.Helper()
	return v.WsScalar(t, `SELECT string_agg(a->>'kind' || '=' || (a->>'outcome'), ',' ORDER BY a->>'kind')
		FROM assurance_deal_receipt r, jsonb_array_elements(r.verdict_actions) a WHERE r.deal_id = $1`, v.deal)
}

func (v *dealVerdictEnv) undo(t *testing.T, correction string) {
	t.Helper()
	var id ids.UUID
	err := database.WithWorkspaceTx(v.Admin(), v.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(v.Admin(), `SELECT id FROM deal_correction
			WHERE deal_id = $1 AND correction = $2 AND reversed_at IS NULL`, v.deal, correction).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Deals.RevertCorrection(v.owner(t), id, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMarginceSetsWhatTheCustomerSaidAndAnUndoHoldsItBack(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)

	if got := v.field(t, "expected_close_date"); got != v.lane.date {
		t.Fatalf("close date = %q, want the date the customer named %s", got, v.lane.date)
	}
	if got := v.field(t, "amount_minor"); got != "4500000" {
		t.Fatalf("amount = %s, want 45,000.00 EUR", got)
	}
	if got := v.field(t, "win_probability_override") + "/" + v.field(t, "win_probability_set_by"); got != "80/margince" {
		t.Fatalf("probability = %s, want 80 set by Margince", got)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND actor_id = $2 AND on_behalf_of = $3`,
		v.deal, marginceActorID, v.Rep1); n != 3 {
		t.Errorf("audit rows by Margince for the owner = %d, want one per change", n)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.subject = 'Margince updated this deal'`, v.deal); n != 1 {
		t.Errorf("timeline notes = %d, want one for the night", n)
	}
	receipts := attentionReceipts{svc: approvals.NewService(InstallationDB(v.Pool)), deals: v.Deals}
	recent, err := receipts.Recent(v.owner(t), v.now.Add(72*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !dealReceiptNamed(recent, "Changed the amount", "45,000 EUR a year") || !dealReceiptNamed(recent, "win probability", "80%") {
		t.Fatalf("Home three days on = %+v, want the amount with its quote and the probability, each with Undo", recent)
	}

	v.run(t)
	if v.lane.calls != 1 {
		t.Fatalf("model calls after a second night with nothing new = %d, want 1", v.lane.calls)
	}

	v.undo(t, deals.VerdictProbabilityCorrection)
	if got := v.field(t, "win_probability_override"); got != "" {
		t.Fatalf("after Undo the probability override = %q, want the stage's again", got)
	}
	v.run(t)
	if got := v.field(t, "win_probability_override"); got != "" || !strings.Contains(v.outcomes(t), "set_probability=dropped:undo_hold") {
		t.Fatalf("the night after an Undo: override %q, outcomes %s; want the change held back", got, v.outcomes(t))
	}

	v.now = v.now.Add(time.Hour)
	v.note = v.logNote(t, "They confirmed again: yes.")
	v.run(t)
	if got := v.field(t, "win_probability_override"); got != "80" {
		t.Fatalf("after a new note the probability = %q, want Margince to set it again", got)
	}
}

func TestASwitchedOffKindIsLeftAlone(t *testing.T) {
	v := setupDealVerdict(t)
	if _, err := approvals.NewService(InstallationDB(v.Pool)).SetAutoApply(v.owner(t), deals.VerdictAmountCorrection, false); err != nil {
		t.Fatal(err)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "4000000" {
		t.Fatalf("amount = %s, want it untouched with the switch off", got)
	}
	if got := v.field(t, "win_probability_override"); got != "80" {
		t.Fatalf("probability = %q, want the other kinds still applied", got)
	}
	if !strings.Contains(v.outcomes(t), "set_amount=dropped:switch_off") {
		t.Fatalf("outcomes = %s, want the amount recorded as switched off", v.outcomes(t))
	}
}

func TestAColleaguesEditAfterTheEvidenceWins(t *testing.T) {
	v := setupDealVerdict(t)
	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the colleague's 30,000.00 EUR kept", got)
	}
	if !strings.Contains(v.outcomes(t), "set_amount=dropped:human_edit") {
		t.Fatalf("outcomes = %s, want the amount dropped for the colleague's edit", v.outcomes(t))
	}
}

func dealReceiptNamed(receipts []attention.Receipt, parts ...string) bool {
	for _, receipt := range receipts {
		matched := receipt.Undo != nil
		for _, part := range parts {
			matched = matched && strings.Contains(receipt.Summary, part)
		}
		if matched {
			return true
		}
	}
	return false
}
