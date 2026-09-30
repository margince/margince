// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// rowFor is a readiness row for site whose record measured binding with status.
func rowFor(site string, binding ai.ProviderConfig, status string) ReadinessRow {
	rec := Record{Task: string(ai.TaskSummarize), Provider: binding.Provider, ServedModel: binding.Model, EnvClass: string(ai.ProfileCloudFrontier)}
	row := ReadinessRow{Site: aitasks.Site{Task: ai.TaskSummarize, Variant: site}, Record: rec, Certified: true}
	switch status {
	case StatusStale:
		row.Standing.Stale = true
	case StatusPartial:
		row.Standing.Pending = 1
	}
	return row
}

// A preset's row names the state of the record that grades each rung: the one
// that answers, and the one a failed call falls to.
func TestAPresetRowGivesTheFirstRungAndTheFallbackTheirOwnState(t *testing.T) {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	scenarios := []Scenario{{Task: string(ai.TaskSummarize), Site: "brief"}, {Task: string(ai.TaskSummarize), Site: "long"}}
	for name, tc := range map[string]struct {
		tiers        map[ai.Tier]ai.ProviderConfig
		rows         []ReadinessRow
		first, fallb string
	}{
		"first current, fallback absent": {
			map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateB},
			[]ReadinessRow{rowFor("brief", candidateA, StatusCurrent), rowFor("long", candidateA, StatusCurrent)},
			StatusCurrent, StatusAbsent,
		},
		"the worst site decides": {
			map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateB},
			[]ReadinessRow{
				rowFor("brief", candidateA, StatusCurrent), rowFor("long", candidateA, StatusStale),
				rowFor("brief", candidateB, StatusCurrent), rowFor("long", candidateB, StatusPartial),
			},
			StatusStale, StatusPartial,
		},
		"a site without a row is absent": {
			map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateB},
			[]ReadinessRow{rowFor("brief", candidateA, StatusCurrent)},
			StatusAbsent, StatusAbsent,
		},
		"the same model is no fallback": {
			map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA, ladder[1]: candidateA},
			nil, StatusAbsent, RungSameModel,
		},
		"a one-rung ladder has none": {
			map[ai.Tier]ai.ProviderConfig{ladder[0]: candidateA},
			nil, StatusAbsent, RungNone,
		},
	} {
		t.Run(name, func(t *testing.T) {
			routing := ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: tc.tiers}
			got := presetRungState(routing, ai.TaskSummarize, scenarios, tc.rows)
			if got.FirstState != tc.first || got.FallbackState != tc.fallb {
				t.Errorf("first %q, fallback %q; want %q and %q", got.FirstState, got.FallbackState, tc.first, tc.fallb)
			}
			if got.FirstModel != candidateA.Model || got.FirstTier != ladder[0] {
				t.Errorf("first rung = %s · %s, want %s · %s", got.FirstTier, got.FirstModel, ladder[0], candidateA.Model)
			}
		})
	}
}
