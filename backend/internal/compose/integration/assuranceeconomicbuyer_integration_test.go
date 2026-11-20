// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
)

// A stored role is free text, so a seat typed " Economic_Buyer " is the deal's
// economic buyer and the input check raises no no_economic_buyer finding.
func TestTheInputCheckReadsAnEconomicBuyerInAnyCase(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Buyer typed by hand", pipeline, open, nil)
	buyer := e.SeedContact(t, "Ute Sommer", nil)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, ' Economic_Buyer ', 'manual', 'human:x')`, buyer, deal)

	now := clocktest.Now(t)
	var subjects []assurance.Subject
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		subjects, err = compose.AssuranceSubjects(e.Admin(), tx, now, compose.AssuranceCoverage(e.Admin(), tx, now))
		return err
	}); err != nil {
		t.Fatalf("reading the subjects: %v", err)
	}
	for _, s := range subjects {
		if s.DealID == deal.String() {
			if !s.HasEconomicBuyer {
				t.Error("a seat stored as \" Economic_Buyer \" is not read as the economic buyer")
			}
			return
		}
	}
	t.Fatal("the deal is not among the input check's subjects")
}
