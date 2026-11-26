// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A failed effect is re-driven by deciding its approval again. The work stays
// the approver's: it runs under the approver's grants as the database holds
// them when it runs, whoever asked for it again.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvedRateWhoseEffectFailed is a GBP proposal Rep1 approved while the
// sheet had moved, so its effect failed and the approval waits, unspent. The
// sheet is then put back, so a re-drive meets a precondition that holds.
func approvedRateWhoseEffectFailed(t *testing.T) (*integration.Env, *approvals.Service, ids.ApprovalID) {
	t.Helper()
	e := integration.Setup(t)
	approver := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	svc := rateSvc(t, e)
	store := deals.NewStore(e.DB(), DealsInstallation())
	setRate := func(rate string) {
		if _, err := store.SetFxRate(e.Admin(), deals.SetFxRateInput{FromCurrency: "GBP", Rate: rate}); err != nil {
			t.Fatalf("setting GBP to %s: %v", rate, err)
		}
	}
	setRate("1.0")
	id := stageProposal(approver, t, svc, fxRateProposalKind, fxRateTargetType, e.WS,
		map[string]string{"from_currency": "GBP", "rate": "1.2", "expected_prior_rate": "1.0"}, "GBP")
	setRate("1.1")
	if _, err := svc.Decide(approver, id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("approving over a moved sheet → %v, want the effect refused", err)
	}
	setRate("1.0")
	return e, svc, id
}

func TestARedrivenEffectActsForTheApproverNotForWhoeverRedrivesIt(t *testing.T) {
	e, svc, id := approvedRateWhoseEffectFailed(t)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)

	if _, err := svc.Decide(colleague, id, true, nil); err != nil {
		t.Fatalf("re-driving the effect: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'fx_rate'
		AND actor_id = 'agent:rate-refresh' AND on_behalf_of = $1`, e.Rep1); n != 1 {
		t.Errorf("%d rate writes acted for the approver, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'fx_rate'
		AND actor_id = 'agent:rate-refresh' AND on_behalf_of = $1`, e.Rep2); n != 0 {
		t.Errorf("%d rate writes acted for the colleague who re-drove it, want 0", n)
	}
}

func TestARedrivenEffectIsRefusedOnceTheApproverWasDemoted(t *testing.T) {
	e, svc, id := approvedRateWhoseEffectFailed(t)
	e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.Rep1)

	if _, err := svc.Decide(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), id, true, nil); err == nil {
		t.Fatal("the effect ran for an approver whose grants were taken away before it ran")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM fx_rate WHERE from_currency = 'GBP' AND rate = 1.2`); n != 0 {
		t.Error("the refused effect wrote the rate")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'approved' AND consumed_at IS NULL`, id); n != 1 {
		t.Error("the refused effect spent the approval; it must stay approved and unspent")
	}
}
