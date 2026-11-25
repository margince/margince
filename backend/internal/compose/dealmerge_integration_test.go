// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deal merge over a real Postgres, with offers and stakeholders written
// through the stores that own them.

import (
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// twoDealsOnOneCompany seeds the merge's usual pair: a source and a target
// that name the same company.
func twoDealsOnOneCompany(t *testing.T, e *integration.Env) (company ids.UUID, source, target ids.DealID) {
	t.Helper()
	company = e.SeedCompany(t, "Merge Works GmbH", nil)
	source = ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Duplicate", company))
	target = ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Original", company))
	return company, source, target
}

func addStakeholder(t *testing.T, e *integration.Env, deal ids.DealID, contact ids.UUID) {
	t.Helper()
	contactID := ids.From[ids.ContactKind](contact)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contactID, DealID: &deal, Source: "manual",
	}); err != nil {
		t.Fatalf("adding a stakeholder: %v", err)
	}
}

func TestDealMergeMovesTheFactsAndLeavesThePathOnTheLoser(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	_, source, target := twoDealsOnOneCompany(t, e)
	shared, only := e.SeedContact(t, "Shared Buyer", nil), e.SeedContact(t, "Only On Duplicate", nil)
	addStakeholder(t, e, source, shared)
	addStakeholder(t, e, source, only)
	addStakeholder(t, e, target, shared)
	offer, err := e.Deals.CreateOffer(admin, source, deals.CreateOfferInput{Currency: "EUR", Source: "manual"})
	if err != nil {
		t.Fatalf("create offer on the duplicate: %v", err)
	}
	historyBefore := e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, source)
	survivorHistory := e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, target)

	survivor, err := e.Deals.MergeDeal(admin, source, target, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if ids.UUID(survivor.Id) != target.UUID {
		t.Fatalf("survivor = %s, want the target %s", survivor.Id, target)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM offer WHERE deal_id = $1 AND id = $2`, target, ids.UUID(offer.Id)); n != 1 {
		t.Errorf("the duplicate's offer sits on the survivor %d times, want once", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE deal_id = $1`, source); n != 0 {
		t.Errorf("%d stakeholder edges still name the merged-away deal", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE deal_id = $1 AND archived_at IS NULL`, target); n != 2 {
		t.Errorf("the survivor has %d live stakeholders, want 2: the shared buyer once and the duplicate's own", n)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, source); got != historyBefore {
		t.Errorf("the merged-away deal has %d stage history rows, want its own %d: the path stays with the deal that took it", got, historyBefore)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, target); got != survivorHistory {
		t.Errorf("the survivor has %d stage history rows, want its own %d: a copied history reads as a younger deal", got, survivorHistory)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NOT NULL AND merged_into_id = $2`, source, target); n != 1 {
		t.Errorf("the merged-away deal is not archived with its pointer to the survivor")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'merge' AND entity_id IN ($1, $2)`, source, target); n != 2 {
		t.Errorf("merge audit rows = %d, want one on each deal", n)
	}
}

func TestDealMergeRefusesWhatCannotJoin(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	_, source, target := twoDealsOnOneCompany(t, e)

	var self *recordmerge.MergeSelfError
	if _, err := e.Deals.MergeDeal(admin, source, source, nil); !errors.As(err, &self) {
		t.Errorf("a self-merge → %v, want MergeSelfError", err)
	}

	otherCompany := e.SeedCompany(t, "Another Buyer AG", nil)
	elsewhere := ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Elsewhere", otherCompany))
	var refused *deals.MergeRefusedError
	if _, err := e.Deals.MergeDeal(admin, elsewhere, target, nil); !errors.As(err, &refused) || refused.Code != "different_company" {
		t.Errorf("deals of two companies → %v, want MergeRefusedError different_company", err)
	}

	e.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, elsewhere)
	if _, err := e.Deals.MergeDeal(admin, elsewhere, target, nil); !errors.As(err, &refused) {
		t.Errorf("a deal under legal hold → %v, want a refusal", err)
	}

	if _, err := e.Deals.MergeDeal(admin, source, target, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	var already *recordmerge.AlreadyMergedError
	if _, err := e.Deals.MergeDeal(admin, source, target, nil); !errors.As(err, &already) || already.IntoID != target.UUID {
		t.Errorf("merging the same source again → %v, want AlreadyMergedError naming the survivor", err)
	}
	third := ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Third", e.SeedCompany(t, "Third Co", nil)))
	var dead *recordmerge.MergedTargetError
	if _, err := e.Deals.MergeDeal(admin, third, source, nil); !errors.As(err, &dead) {
		t.Errorf("merging into a merged-away deal → %v, want MergedTargetError", err)
	}
}

func TestDealMergeMovesTheLosersPinnedOffer(t *testing.T) {
	e := integration.Setup(t)
	_, source, target := twoDealsOnOneCompany(t, e)
	offer, err := e.Deals.CreateOffer(e.Admin(), source, deals.CreateOfferInput{Currency: "EUR", Source: "manual"})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	e.WsExec(t, `UPDATE deal SET arr_source_offer_id = $2, expected_arr_minor = 1200, currency = 'EUR' WHERE id = $1`, source, ids.UUID(offer.Id))

	if _, err := e.Deals.MergeDeal(e.Admin(), source, target, nil); err != nil {
		t.Fatalf("merging a deal whose recurring revenue is pinned to its own offer: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM offer WHERE id = $1 AND deal_id = $2`, ids.UUID(offer.Id), target); n != 1 {
		t.Errorf("the pinned offer did not move to the survivor")
	}
}

func TestAMergedAwayDealCannotBeRestored(t *testing.T) {
	e := integration.Setup(t)
	_, source, target := twoDealsOnOneCompany(t, e)
	if _, err := e.Deals.MergeDeal(e.Admin(), source, target, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	err := restoreIn(t, e, func(tx pgx.Tx) (storekit.RestoreReport, error) {
		return e.Deals.RestoreDealTx(e.Admin(), tx, source, nil, unarchiveWith(nil))
	})
	var refusal *storekit.RestoreRefusal
	if !errors.As(err, &refusal) || refusal.Reason != storekit.RestoreMerged {
		t.Fatalf("restoring a merged-away deal → %v, want a RestoreMerged refusal", err)
	}
}

// Two merges of one pair in opposite directions run under one pair lock, so one
// wins and the other finds its source or target already gone.
func TestTwoOppositeDealMergesLeaveOneWinner(t *testing.T) {
	e := integration.Setup(t)
	_, a, b := twoDealsOnOneCompany(t, e)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, pair := range [][2]ids.DealID{{a, b}, {b, a}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = e.Deals.MergeDeal(e.Admin(), pair[0], pair[1], nil)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("opposite merges answered %v and %v, want exactly one success", results[0], results[1])
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE merged_into_id IS NOT NULL AND id IN ($1, $2)`, a, b); n != 1 {
		t.Errorf("%d deals are merged away, want exactly 1", n)
	}
}
