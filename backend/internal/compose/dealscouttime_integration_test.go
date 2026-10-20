// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestScoutUsesTheSourceDateInsteadOfDetection(t *testing.T) {
	for _, age := range []int{2, 120} {
		t.Run(time.Duration(age).String(), func(t *testing.T) {
			e := setupScout(t)
			e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
			company := e.SeedCompany(t, "Maple Studio", nil)
			source, buyer := e.buyingEmail(t, company, e.daysAgo(age))
			e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
			e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
			want := 1
			if age > 90 {
				want = 0
			}
			if pass := e.pass(t); pass.Raised != want {
				t.Fatalf("source age %d: raised %d, want %d", age, pass.Raised, want)
			}
			for _, suggestion := range e.suggestions(e.signalReader(), t, company) {
				for _, evidence := range suggestion.Evidence {
					if !evidence.OccurredAt.Equal(e.daysAgo(age)) {
						t.Fatalf("evidence date = %v, want source date %v", evidence.OccurredAt, e.daysAgo(age))
					}
				}
			}
		})
	}
}

func TestSuggestionWriterRefusesDetectionTimeAsEvidenceTime(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source := e.email(t, "An old project", "inbound", company, e.daysAgo(120))
	signal := e.signal(t, "new_opportunity", company, source, e.now, ids.Nil)
	raised, err := e.record(e.system(), t, deals.SuggestionDraft{
		CompanyID: company, NameHint: deals.HintOpportunitySignaled, Confidence: .7,
		Evidence: []deals.SuggestionEvidence{{Kind: deals.EvidenceSignal, SignalID: &signal, OccurredAt: e.now}},
	})
	if err != nil || raised {
		t.Fatalf("record with processing date = %v, %v; want refusal without write", raised, err)
	}
}
