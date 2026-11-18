// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

func TestThePromptStatesTheLimitsTheGuardsHold(t *testing.T) {
	t.Parallel()
	fence := promptfence.New()
	if system := SystemFor(fence, ""); !strings.Contains(system, fmt.Sprintf("at most %d months after today", maxMonthsAhead)) {
		t.Errorf("system prompt does not name the %d-month close-date limit:\n%s", maxMonthsAhead, system)
	}
	in := guardInput()
	limit := fmt.Sprintf("Highest probability a raise may set: %d%%", in.StageProbability)
	if prompt := Prompt(fence, in); !strings.Contains(prompt, limit) {
		t.Fatalf("prompt does not state %q:\n%s", limit, prompt)
	}
	for probability, want := range map[int]string{in.StageProbability: OutcomeApply, in.StageProbability + 1: DroppedAboveStage} {
		got, err := Decide(in, guardState(), Payload{Actions: []Action{probabilityAction(probability)}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Outcome != want {
			t.Errorf("a raise to %d%% under the stated limit = %q, want %q", probability, got[0].Outcome, want)
		}
	}
}

func TestAPrivateMailReachesThePromptByDirectionAndDateOnly(t *testing.T) {
	t.Parallel()
	in := guardInput()
	in.PrivateMail = []PrivateMail{
		{Direction: "inbound", OccurredAt: time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)},
		{Direction: "outbound", OccurredAt: time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)},
	}
	prompt := Prompt(promptfence.New(), in)
	for _, want := range []string{"A private email from the customer on 2026-08-03.", "A private email to the customer on 2026-08-04."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not state %q:\n%s", want, prompt)
		}
	}
}
