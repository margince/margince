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

// A stored role is free text, so a seat typed " Economic_Buyer " or padded with
// a tab is the deal's economic buyer. It raises no no_economic_buyer finding.
func TestTheInputCheckReadsAnEconomicBuyerInAnyCase(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	typedOn := map[string]string{}
	for _, typed := range []string{" Economic_Buyer ", "\tEconomic_Buyer\n", "\u00a0economic_buyer"} {
		deal := e.SeedDeal(t, "Buyer typed by hand", pipeline, open, nil)
		buyer := e.SeedContact(t, "Ute Sommer", nil)
		e.WsExec(t, `INSERT INTO relationship (kind, contact_id, deal_id, role, source, captured_by)
			VALUES ('deal_stakeholder', $1, $2, $3, 'manual', 'human:x')`, buyer, deal, typed)
		typedOn[deal.String()] = typed
	}

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
		typed, ok := typedOn[s.DealID]
		if !ok {
			continue
		}
		delete(typedOn, s.DealID)
		if !s.HasEconomicBuyer {
			t.Errorf("a seat stored as %q is not read as the economic buyer", typed)
		}
	}
	if len(typedOn) > 0 {
		t.Fatalf("deals missing from the input check's subjects: %v", typedOn)
	}
}
