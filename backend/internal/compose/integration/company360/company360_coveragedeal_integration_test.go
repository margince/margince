// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// openDeals is the fixture the coverage cases share: an account and its deals.
type openDeals struct {
	company, staffed, empty ids.UUID
	pipeline                ids.PipelineID
	openStage               ids.StageID
}

// twoOpenDeals seeds an account with an older deal that has an economic buyer
// and a newer one with no seat.
func twoOpenDeals(t *testing.T, e *integration.Env) openDeals {
	t.Helper()
	company := e.SeedCompany(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	staffed := e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	empty := e.SeedDeal(t, "Fleet 2027", pipeline, openStage, nil)
	buyer := e.SeedContact(t, "Ute Sommer", nil)
	employ(t, e, buyer, company, "Procurement")
	e.WsExec(t, `UPDATE deal SET company_id = $1, updated_at = now() - interval '2 days' WHERE id = $2`, company, staffed)
	e.WsExec(t, `UPDATE deal SET company_id = $1, updated_at = now() WHERE id = $2`, company, empty)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x')`, buyer, staffed)
	return openDeals{company: company, staffed: staffed, empty: empty, pipeline: pipeline, openStage: openStage}
}

// With two open deals the reader picks the committee; unnamed, it is the
// newest deal's, and the named one is the one described.
func TestCoverageReadsTheCommitteeOfTheDealNamed(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := company360Service(e)
	seeded := twoOpenDeals(t, e)
	company, staffed, empty := seeded.company, seeded.staffed, seeded.empty

	unnamed, err := svc.Coverage(ctx, ids.CompanyID{UUID: company}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(unnamed.Deals) != 2 || unnamed.SelectedDealId == nil || ids.UUID(*unnamed.SelectedDealId) != empty {
		t.Fatalf("unnamed read selected %v of %d deals, want the newest %s", unnamed.SelectedDealId, len(unnamed.Deals), empty)
	}

	pick := ids.From[ids.DealKind](staffed)
	named, err := svc.Coverage(ctx, ids.CompanyID{UUID: company}, &pick)
	if err != nil {
		t.Fatal(err)
	}
	if named.SelectedDealId == nil || ids.UUID(*named.SelectedDealId) != staffed {
		t.Fatalf("deal_id %s selected %v", staffed, named.SelectedDealId)
	}
	if named.Committee == nil || len(named.Committee.Seats) != 1 || named.Committee.Seats[0].Role != "economic_buyer" {
		t.Fatalf("committee = %+v, want the named deal's one economic buyer", named.Committee)
	}
}

// A deal_id outside the account's visible open deals is not found. Another
// account's deal and one that does not exist answer the same.
func TestCoverageRefusesADealOutsideTheAccountsOpenDeals(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := company360Service(e)
	seeded := twoOpenDeals(t, e)
	elsewhere := e.SeedDeal(t, "Someone else's deal", seeded.pipeline, seeded.openStage, nil)

	for name, deal := range map[string]ids.UUID{"another account's": elsewhere, "an unknown": ids.NewV7()} {
		pick := ids.From[ids.DealKind](deal)
		if _, err := svc.Coverage(ctx, ids.CompanyID{UUID: seeded.company}, &pick); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s deal answered %v, want not found", name, err)
		}
	}
}
