// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var (
	sweepNotedDeal  = ids.MustParse("0198f3a1-7c42-7e0b-9d51-2a6f4b8c3b01")
	sweepMissedDeal = ids.MustParse("0198f3a1-7c42-7e0b-9d51-2a6f4b8c3b02")
)

// sweepSurface answers the sweep's list read with two deals, sealed as the
// registry seals it, and acknowledges every note it is handed.
type sweepSurface struct{}

func (sweepSurface) Specs() []mcp.ToolSpec                         { return agentLoopRegistry() }
func (sweepSurface) Runnable(context.Context) []mcp.ToolSpec       { return agentLoopRegistry() }
func (sweepSurface) Explain(context.Context, string, error) string { return "refused" }

func (sweepSurface) Invoke(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	data := json.RawMessage(`{}`)
	if name == "whats_slipping_this_week" {
		// Long evidence puts the second deal past the trace's cut of an answer.
		padding := []agents.SlippingEvidence{{Source: "last_activity_at", Snippet: strings.Repeat("quiet ", 400)}}
		listed, err := json.Marshal(agents.WhatsSlippingResult{Deals: []agents.SlippingDealItem{
			{Rank: 1, DealID: sweepNotedDeal, Name: "Acme", Evidence: padding},
			{Rank: 2, DealID: sweepMissedDeal, Name: "Globex", Evidence: padding},
		}})
		if err != nil {
			return nil, err
		}
		data = listed
	}
	return json.Marshal(agents.Envelope{Evidence: []agents.EvidenceRef{}, Warnings: []agents.Warning{}, Data: data})
}

// claimingBrain plays its replies in order and keeps every request it was sent.
type claimingBrain struct {
	replies  []string
	requests []model.Request
}

func (b *claimingBrain) Complete(_ context.Context, req model.Request) (model.Response, runner.Meta, error) {
	b.requests = append(b.requests, req)
	reply := `{"final":{"summary":"I logged a note on every deal"}}`
	if len(b.requests) <= len(b.replies) {
		reply = b.replies[len(b.requests)-1]
	}
	return model.Response{Text: reply, OutputTokens: 10}, runner.Meta{}, nil
}

func (*claimingBrain) PromptWindow() int { return runner.MinimumPromptWindow }

const (
	sweepListStep  = `{"tool":"whats_slipping_this_week","args":{}}`
	sweepClaimStep = `{"final":{"summary":"I logged a note on every deal"}}`
)

func sweepNoteStep(deal ids.UUID) string {
	return fmt.Sprintf(`{"tool":"log_activity","args":{"kind":"note","subject":"Quiet","body":"No touch in weeks.",`+
		`"links":[{"entity_type":"deal","entity_id":%q}]}}`, deal)
}

func runSweep(t *testing.T, agent string, brain *claimingBrain) runner.Result {
	t.Helper()
	spec, ok := ScheduledAgentSpecByName(agent)
	if !ok {
		t.Fatalf("%s is not a scheduled agent", agent)
	}
	res, err := runner.New(sweepSurface{}, brain).Run(context.Background(), runner.Job{
		Goal: spec.Goal, Tools: spec.Tools, Budget: runner.Budget{MaxSteps: 8}, Outcome: outcomeCheckFor(spec),
	})
	if err != nil {
		t.Fatalf("the run faulted: %v", err)
	}
	return res
}

func lastPrompt(brain *claimingBrain) string {
	msgs := brain.requests[len(brain.requests)-1].Messages
	return msgs[len(msgs)-1].Content
}

func TestASweepClaimingEveryDealIsSentBackForTheDealWithNoNote(t *testing.T) {
	brain := &claimingBrain{replies: []string{
		sweepListStep, sweepNoteStep(sweepNotedDeal), sweepClaimStep, sweepNoteStep(sweepMissedDeal), sweepClaimStep,
	}}
	res := runSweep(t, "overnight_at_risk_sweep", brain)

	if res.Outcome != runner.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s), want completed once the missing note exists", res.Outcome, res.DegradeReason)
	}
	if len(brain.requests) != 5 {
		t.Fatalf("model calls = %d, want 5: the first claim is refused, the second accepted", len(brain.requests))
	}
	refusal := brain.requests[3].Messages[len(brain.requests[3].Messages)-1].Content
	if !strings.Contains(refusal, sweepMissedDeal.String()) || strings.Contains(refusal, sweepNotedDeal.String()) {
		t.Errorf("the refusal must name only the deal with no note: %q", refusal)
	}
	if !strings.Contains(refusal, "not accepted") {
		t.Errorf("the refusal must say the final answer was not accepted: %q", refusal)
	}
}

func TestASweepThatNotedEveryDealIsAcceptedAtOnce(t *testing.T) {
	brain := &claimingBrain{replies: []string{
		sweepListStep, sweepNoteStep(sweepNotedDeal), sweepNoteStep(sweepMissedDeal), sweepClaimStep,
	}}
	res := runSweep(t, "overnight_at_risk_sweep", brain)

	if res.Outcome != runner.OutcomeCompleted || len(brain.requests) != 4 {
		t.Fatalf("outcome = %q after %d calls, want completed after 4", res.Outcome, len(brain.requests))
	}
}

func TestASweepThatNeverWritesTheNoteEndsOnItsStepBudget(t *testing.T) {
	brain := &claimingBrain{replies: []string{sweepListStep, sweepNoteStep(sweepNotedDeal)}}
	res := runSweep(t, "overnight_at_risk_sweep", brain)

	if res.Outcome != runner.OutcomeDegraded || !strings.Contains(res.DegradeReason, "step budget") {
		t.Fatalf("outcome = %q (%s), want the step budget's degrade", res.Outcome, res.DegradeReason)
	}
	if !strings.Contains(lastPrompt(brain), sweepMissedDeal.String()) {
		t.Errorf("every refusal names the missing deal: %q", lastPrompt(brain))
	}
}

func TestABriefsFinalAnswerIsNeverSentBack(t *testing.T) {
	brain := &claimingBrain{replies: []string{sweepClaimStep}}
	res := runSweep(t, "morning_brief", brain)

	if res.Outcome != runner.OutcomeCompleted || len(brain.requests) != 1 {
		t.Fatalf("outcome = %q after %d calls, want the brief's first answer accepted", res.Outcome, len(brain.requests))
	}
}

func TestOnlyTheSweepCarriesAnOutcomeCheck(t *testing.T) {
	for _, entry := range runner.Catalog() {
		spec, ok := ScheduledAgentSpecByName(entry.Name)
		if !ok {
			t.Fatalf("%s is not a scheduled agent", entry.Name)
		}
		check := outcomeCheckFor(spec)
		if want := spec.Name == "overnight_at_risk_sweep"; (check != nil) != want {
			t.Errorf("%s: carries an outcome check = %v, want %v", spec.Name, check != nil, want)
		}
	}
}
