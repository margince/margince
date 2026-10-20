// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The account's won count rides the query that sums won_lifetime, so the two
// figures describe the same deals. An archived deal is in neither. A deal whose
// amount is masked from the reader still counts.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAccountsWonCountCountsTheDealsTheWonTotalCovers(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, mine, _ := seedTwoPricedDeals(t, e, "won")
	companyID := ids.From[ids.CompanyKind](company)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, maskedRep())

	// The reader may price only its own deal: the count is about existence, so
	// the masked one counts while the total leaves its amount out.
	both, err := svc.Assemble(rep, companyID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if both.Deals == nil {
		t.Fatalf("the deals band was withheld entirely (sections_omitted=%v)", both.SectionsOmitted)
	}
	if both.Deals.WonCount != 2 {
		t.Errorf("won_count = %d, want 2 — the masked deal is still a won deal", both.Deals.WonCount)
	}
	if both.Deals.LostCount != 0 {
		t.Errorf("lost_count = %d, want 0 on an account with no lost deal", both.Deals.LostCount)
	}

	e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, mine)
	one, err := svc.Assemble(rep, companyID)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if one.Deals == nil {
		t.Fatalf("the deals band was withheld entirely (sections_omitted=%v)", one.SectionsOmitted)
	}
	if one.Deals.WonCount != 1 {
		t.Errorf("won_count = %d after archiving one of two won deals, want 1", one.Deals.WonCount)
	}
	if got := one.Deals.WonLifetime.AmountMinor; got == nil || *got != 0 {
		t.Errorf("won_lifetime = %v, want 0: the one remaining won deal is masked from this reader", got)
	}
}
