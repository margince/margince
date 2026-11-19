// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// The close-date and next-step rules ask the deal's viability decision first.
// Where it judged the deal, its reasons decide; where it could not, the
// record's own answer stands.
func TestTheWalkAsksTheViabilityDecisionFirst(t *testing.T) {
	t.Parallel()
	weak := healthy()
	weak.ExpectedClose, weak.CloseProvisional = new(at(time.April, 30)), true
	weak.HasNextStep = false
	all := []string{TypeClosePast, TypeCloseUnconfirmed, TypeNoNextStep}

	for _, tc := range []struct {
		name    string
		future  bool
		judged  *SubjectViability
		raised  []string
		refused []string
	}{
		{
			name: "unsupported for both reasons",
			judged: &SubjectViability{Outcome: viability.OutcomeUnsupported, Reasons: []viability.Reason{
				viability.ReasonCloseDateUnsupported, viability.ReasonNoNextStep,
			}},
			raised: all,
		},
		{
			// The customer asked to wait past the close date: the wait is
			// the next step, and only the date lacks support.
			name: "the customer asked to wait",
			judged: &SubjectViability{Outcome: viability.OutcomeUnsupported, Reasons: []viability.Reason{
				viability.ReasonCloseDateUnsupported,
			}},
			raised: []string{TypeClosePast, TypeCloseUnconfirmed}, refused: []string{TypeNoNextStep},
		},
		{
			// A confirmed future date and a meeting the customer accepted,
			// which viability counts as the next step.
			name:    "judged with nothing lacking",
			future:  true,
			judged:  &SubjectViability{Outcome: viability.OutcomeWaiting, Reasons: []viability.Reason{viability.ReasonMeetingConfirmed}},
			refused: all,
		},
		{
			name:   "the mailbox was not read",
			judged: &SubjectViability{Outcome: viability.OutcomeUnknown, Reasons: []viability.Reason{viability.ReasonCoverageUnknown}},
			raised: all,
		},
		{name: "never assessed", raised: all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deal := weak
			if tc.future {
				deal.ExpectedClose, deal.CloseProvisional = new(at(time.June, 30)), false
			}
			deal.Viability = tc.judged
			pass := askEveryRule(at(time.May, 14), []Subject{deal}, DefaultConfig(), nil)
			var types []string
			for _, finding := range pass.findings() {
				types = append(types, finding.Type)
			}
			for _, want := range tc.raised {
				if !slices.Contains(types, want) {
					t.Errorf("the walk raised %v, want %s among them", types, want)
				}
			}
			for _, refused := range tc.refused {
				if slices.Contains(types, refused) {
					t.Errorf("the walk raised %s although the viability decision answered it otherwise", refused)
				}
			}
		})
	}
}

// The viability decision answers the close-date and next-step rules and no
// others; the money, buyer and stage rules keep filing tasks.
func TestTheViabilityDecisionAnswersOnlyTheCloseDateAndNextStepRules(t *testing.T) {
	t.Parallel()
	got := ViabilityAnsweredTypes()
	want := []string{TypeClosePast, TypeCloseUnconfirmed, TypeNoNextStep}
	if !slices.Equal(got, want) {
		t.Fatalf("the viability decision answers %v, want %v", got, want)
	}
}
