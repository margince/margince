// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
)

// perf5FirstTokenBudget is PERF-5: an AI baseline action shows its first token
// within 1.5 s.
const perf5FirstTokenBudget = 1500 * time.Millisecond

// aiDraftReply is a draft the contact drafter accepts as the model's own.
const aiDraftReply = `{"subject":"Next steps","body":"Hello,\n\nShall we pick this up next week?"}`

// TestAIDraftBudget times POST /v1/contacts/{id}/draft-email with an offline
// model that answers at once. The endpoint answers whole rather than as a
// stream, so its answer is its first token, and the time is the product's
// own share of the budget: the model's latency is not in it.
func TestAIDraftBudget(t *testing.T) {
	spec := recordBenchSpec
	model := ai.NewFakeClient().Script(slices.Repeat([]string{aiDraftReply}, spec.warmups+spec.sample)...)
	e := apptest.SetupAppWithOptions(t, compose.WithContactDraft(model))
	e.BootstrapWorkspace(t)
	contact := createBenchContact(t, e)

	stats, err := benchRuns("ai_draft_contact", perf5FirstTokenBudget, spec, func() error {
		var draft crmcontracts.CompanyEmailDraft
		if status := e.Call(t, http.MethodPost, "/v1/contacts/"+contact+"/draft-email", nil, nil, &draft); status != http.StatusOK {
			return fmt.Errorf("draft-email answered %d", status)
		}
		// The floor answers when the model's reply is refused, and timing it
		// would measure a path with no model in it.
		if draft.GeneratedBy != crmcontracts.WrittenByModel {
			return fmt.Errorf("the draft was written by %q, not the model", draft.GeneratedBy)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("perfbench: ai_draft_contact p50=%s p95=%s p99=%s budget=%s samples=%d",
		stats.P50, stats.P95, stats.P99, perf5FirstTokenBudget, stats.Samples)
	row := MeasurementFrom("PERF-5", "ai_draft_contact", stats.P50, stats.P95, stats.P99, perf5FirstTokenBudget, stats.Samples)
	row.Note = "the whole draft from an offline model that answers at once: the product's share, without the model's latency"
	WritePerfRecord(t, "bench-ai", benchPostgresVersion(e.Owner), []BudgetMeasurement{row})
	if err := (search.BenchReport{Queries: []search.QueryStats{stats}}).Gate(); err != nil {
		t.Fatalf("PERF-5 budget gate is red: %v", err)
	}
}
