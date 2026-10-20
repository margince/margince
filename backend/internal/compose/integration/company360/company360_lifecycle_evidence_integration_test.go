// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLifecycleConflictCitesTheSourceInsteadOfTheExtractionDate(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Cedar Labs", &e.Rep1))
	e.WsExec(t, `UPDATE company SET lifecycle = 'customer' WHERE id = $1`, company.UUID)
	occurred := company360Clock.AddDate(0, 0, -20)
	quote := "We are ending all business with your company."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Body: &quote, OccurredAt: &occurred, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: "contract_ended", CompanyID: company.UUID, Summary: "The account relationship ended.",
			Severity: "warn", Fingerprint: "cedar-company-termination",
			Evidence: []signals.DerivedEvidence{{ActivityID: ids.UUID(source.Id), Snippet: quote, LifecycleScope: "company"}},
		}, occurred.Add(10*24*time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	view, err := company360Service(e).Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, company360SignalPerms), company)
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range *view.Suggestions {
		if string(suggestion.Kind) != "lifecycle_conflict" {
			continue
		}
		if len(suggestion.Evidence) != 1 {
			t.Fatalf("source evidence: %+v", suggestion.Evidence)
		}
		evidence := suggestion.Evidence[0]
		if string(evidence.EntityType) != "activity" || evidence.EntityId != source.Id {
			t.Fatalf("conflict must link the readable source: %+v", evidence)
		}
		if evidence.Quote == nil || *evidence.Quote != quote || evidence.At == nil || !evidence.At.Equal(occurred) {
			t.Fatalf("conflict must cite source words and occurrence: %+v", evidence)
		}
		return
	}
	t.Fatal("account-wide termination had no reviewable conflict")
}
