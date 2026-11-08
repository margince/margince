// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealReceiptChanges reads what one audit row moved on a deal, through the
// receipt read Done for you uses.
func dealReceiptChanges(ctx context.Context, t *testing.T, pool *pgxpool.Pool, audit, deal ids.UUID) []crmcontracts.MagicFieldChange {
	t.Helper()
	svc := newMagicService(pool, approvals.NewService(InstallationDB(pool)), time.Now)
	changes, err := svc.ReceiptChanges(ctx, []magic.ReceiptRecord{{ReceiptID: audit, EntityType: "deal", EntityID: deal}})
	if err != nil {
		t.Fatal(err)
	}
	return changes[audit]
}

func fieldChange(changes []crmcontracts.MagicFieldChange, field string) (crmcontracts.MagicFieldChange, bool) {
	for _, c := range changes {
		if c.Field == field {
			return c, true
		}
	}
	return crmcontracts.MagicFieldChange{}, false
}

func correctionAudit(t *testing.T, e *integration.Env, deal ids.UUID, correction string) ids.UUID {
	t.Helper()
	id, err := ids.Parse(e.WsScalar(t, `SELECT audit_log_id::text FROM deal_correction
		WHERE deal_id = $1 AND correction = $2 ORDER BY applied_at DESC LIMIT 1`, deal, correction))
	if err != nil {
		t.Fatalf("no %s correction on the deal: %v", correction, err)
	}
	return id
}

// requireImageKey proves a write stored key. Its absence from the receipt is
// then the presentation rule at work, not a write that never touched it.
func requireImageKey(t *testing.T, e *integration.Env, audit ids.UUID, key string) {
	t.Helper()
	if e.WsScalar(t, `SELECT (after ? $2)::text FROM audit_log WHERE id = $1`, audit, key) != "true" {
		t.Fatalf("audit row %s did not store %s; the test no longer exercises it", audit, key)
	}
}

func TestAReceiptAmountCarriesTheCurrencyTheDealHeld(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	amountAudit := correctionAudit(t, v.Env, v.deal, deals.VerdictAmountCorrection)

	amount, ok := fieldChange(dealReceiptChanges(v.owner(t), t, v.Pool, amountAudit, v.deal), "amount_minor")
	written, err := strconv.ParseFloat(v.field(t, "amount_minor"), 64)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || amount.Type == nil || *amount.Type != crmcontracts.MagicFieldChangeTypeMoney ||
		amount.After != written || amount.Currency == nil || *amount.Currency != "EUR" {
		t.Fatalf("amount = %+v (found=%t), want %v minor units in EUR", amount, ok, written)
	}

	usd, restated := "USD", int64(5_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{
		Currency: &usd, AmountMinor: &restated,
	}); err != nil {
		t.Fatal(err)
	}
	amount, _ = fieldChange(dealReceiptChanges(v.owner(t), t, v.Pool, amountAudit, v.deal), "amount_minor")
	if amount.Currency != nil {
		t.Fatalf("amount names %s after the deal moved to USD; the earlier amount's currency is not the current one", *amount.Currency)
	}
}

func TestAProbabilityReceiptHidesWhoSetIt(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	audit := correctionAudit(t, v.Env, v.deal, deals.VerdictProbabilityCorrection)
	requireImageKey(t, v.Env, audit, "win_probability_set_by")

	changes := dealReceiptChanges(v.owner(t), t, v.Pool, audit, v.deal)
	if _, leaked := fieldChange(changes, "win_probability_set_by"); leaked {
		t.Errorf("the receipt lists win_probability_set_by: %+v", changes)
	}
	figure, ok := fieldChange(changes, "win_probability_override")
	if !ok || figure.Type == nil || *figure.Type != crmcontracts.MagicFieldChangeTypeNumber {
		t.Errorf("the probability reads %+v (found=%t), want the figure as a number", figure, ok)
	}
}

func TestAnOwnerChangeNamesBothMembers(t *testing.T) {
	v := setupDealVerdict(t)
	next := ids.From[ids.UserKind](v.Rep2)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{OwnerID: &next}); err != nil {
		t.Fatal(err)
	}
	audit, err := ids.Parse(v.WsScalar(t, `SELECT id::text FROM audit_log
		WHERE entity_id = $1 AND after ? 'owner_id' ORDER BY occurred_at DESC LIMIT 1`, v.deal))
	if err != nil {
		t.Fatal(err)
	}

	owner, ok := fieldChange(dealReceiptChanges(v.Admin(), t, v.Pool, audit, v.deal), "owner_id")
	before := v.WsScalar(t, `SELECT display_name FROM app_user WHERE id = $1`, v.Rep1)
	after := v.WsScalar(t, `SELECT display_name FROM app_user WHERE id = $1`, v.Rep2)
	if !ok || owner.Type == nil || *owner.Type != crmcontracts.MagicFieldChangeTypeMember ||
		owner.BeforeLabel == nil || *owner.BeforeLabel != before || owner.AfterLabel == nil || *owner.AfterLabel != after {
		t.Fatalf("owner = %+v (found=%t), want %q → %q", owner, ok, before, after)
	}
}

func TestASilentCloseNamesTheStagesItMovedBetween(t *testing.T) {
	c := setupCloseLost(t)
	from := c.WsScalar(t, `SELECT s.name FROM deal d JOIN stage s ON s.id = d.stage_id WHERE d.id = $1`, c.silent)
	if got := c.run(t); got != 1 {
		t.Fatalf("closed %d deals, want the silent one", got)
	}
	to := c.WsScalar(t, `SELECT s.name FROM deal d JOIN stage s ON s.id = d.stage_id WHERE d.id = $1`, c.silent)
	audit := correctionAudit(t, c.Env, c.silent, deals.CloseLostCorrection)
	requireImageKey(t, c.Env, audit, "win_probability_set_by")

	changes := dealReceiptChanges(c.owner(t), t, c.Pool, audit, c.silent)
	stage, ok := fieldChange(changes, "stage_id")
	if !ok || stage.Type == nil || *stage.Type != crmcontracts.MagicFieldChangeTypeStage ||
		stage.BeforeLabel == nil || *stage.BeforeLabel != from || stage.AfterLabel == nil || *stage.AfterLabel != to {
		t.Fatalf("stage = %+v (found=%t), want %q → %q", stage, ok, from, to)
	}
	if _, leaked := fieldChange(changes, "win_probability_set_by"); leaked {
		t.Errorf("the close lists win_probability_set_by: %+v", changes)
	}
}
