// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The report of what Margince changed, over changes the real nightly pass
// wrote: the owner's open bundle, the Undo of many, and what a colleague may
// read of somebody else's deal.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// standInReason is the reason the stand-in model gives for the close date.
const standInReason = "They will sign then"

func TestOneDealChangedThreeWaysCountsOnceAndItsUndoReportsWhatWasKept(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)

	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Deals != 1 || summary.Closed != 0 || summary.Redated != 1 || summary.Revalued != 1 || summary.Probabilities != 1 {
		t.Fatalf("summary = %+v, want one deal with a new date, amount and probability", summary)
	}
	if len(summary.Bundles) != 1 || len(summary.Bundles[0].AuditLogIDs) != 3 {
		t.Fatalf("bundles = %+v, want one bundle naming the three changes", summary.Bundles)
	}
	bundle := summary.Bundles[0]
	item := crmcontracts.UndoMarginceChangeItem{DealId: openapi_types.UUID(bundle.DealID.UUID), AuditLogIds: wireUUIDs(bundle.AuditLogIDs)}

	stale := bundle.Version - 1
	item.Version = &stale
	refused, err := h.undoBundle(v.owner(t), item)
	if err != nil {
		t.Fatal(err)
	}
	if refused.Outcome != crmcontracts.UndoMarginceChangeResultOutcomeRefused || len(refused.Kept) != 3 ||
		refused.Kept[0].Reason != crmcontracts.UndoMarginceChangeKeptReasonVersionSkew {
		t.Fatalf("undo at a version nobody read = %+v, want every change refused for the version", refused)
	}

	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	item.Version = nil
	partly, err := h.undoBundle(v.owner(t), item)
	if err != nil {
		t.Fatal(err)
	}
	if partly.Outcome != crmcontracts.UndoMarginceChangeResultOutcomePartlyUndone || len(partly.Undone) != 2 ||
		len(partly.Kept) != 1 || partly.Kept[0].Kind == nil || *partly.Kept[0].Kind != deals.VerdictAmountCorrection ||
		partly.Kept[0].Reason != crmcontracts.UndoMarginceChangeKeptReasonChangedSince {
		t.Fatalf("undo after a colleague's amount = %+v, want the date and probability undone and the amount kept", partly)
	}
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the colleague's 30,000.00 EUR kept", got)
	}
	after, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Deals != 1 || after.Revalued != 1 || after.Redated != 0 {
		t.Fatalf("summary after the undo = %+v, want only the kept amount open", after)
	}
}

// colleague reads every deal of the workspace but owns none of them.
func (v *dealVerdictEnv) colleague() context.Context {
	return v.As(v.Rep2, []ids.UUID{v.Team1}, principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
}

func TestAColleagueSeesWhatMarginceChangedButNotWhy(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)
	dealID := ids.From[ids.DealKind](v.deal)

	owned, _, err := h.deals.DealChanges(v.owner(t), dealID, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(owned, func(c deals.CorrectionReceipt) bool { return c.Basis == standInReason }) {
		t.Fatalf("owner's list = %+v, want the reason Margince gave", owned)
	}

	seen, _, err := h.deals.DealChanges(v.colleague(), dealID, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	team, err := h.deals.TeamChanges(v.colleague(), []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for surface, rows := range map[string][]deals.CorrectionReceipt{"deal list": seen, "team list": team.Rows} {
		if len(rows) != 3 {
			t.Fatalf("%s for a colleague = %d changes, want all three", surface, len(rows))
		}
		for _, row := range rows {
			if row.Basis != "" || string(row.Evidence) != "{}" {
				t.Errorf("%s for a colleague carries the reason %q / evidence %s, want neither", surface, row.Basis, row.Evidence)
			}
		}
	}

	agent := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), v.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:change-report", SeatType: principal.SeatFull,
			OnBehalfOf: v.Rep2, UserID: v.Rep2, TeamIDs: []ids.UUID{v.Team1},
			Scopes:      principal.NewScopeSet(principal.ScopeRead),
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}, RowScope: principal.RowScopeAll},
		})
	w := httptest.NewRecorder()
	h.ListDealMarginceChanges(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(agent),
		openapi_types.UUID(v.deal), crmcontracts.ListDealMarginceChangesParams{})
	var page crmcontracts.DealMarginceChanges
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("agent read = %d %s", w.Code, w.Body.String())
	}
	if len(page.Changes) != 3 || strings.Contains(w.Body.String(), standInReason) || strings.Contains(w.Body.String(), "45,000 EUR a year") {
		t.Fatalf("agent for a colleague = %s, want three changes with no reason or quote", w.Body.String())
	}
}
