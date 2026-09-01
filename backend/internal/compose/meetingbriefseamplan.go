// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The plan, mapped from the wire into the agent's vocabulary.
//
// Its own file beside meetingbriefseam.go because it is its own concept and
// because the two together would push that file past the point where a reader
// can hold it: the brief's mapping answers "what is known", this one answers
// "what to do", and they change for different reasons.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agentMeetingPlan maps the plan, keeping every citation.
func agentMeetingPlan(plan *crmcontracts.MeetingPlan) *agents.MeetingPlanResult {
	if plan == nil {
		return nil
	}
	out := &agents.MeetingPlanResult{
		Readiness:  string(plan.Readiness),
		Type:       string(plan.MeetingType.Value),
		Confidence: string(plan.MeetingType.Confidence),
		Advance: agents.MeetingPlanAdvancePart{
			Minimum:  agentBriefLine(plan.Advance.Minimum),
			Best:     agentBriefLine(plan.Advance.Best),
			Fallback: agentBriefLine(plan.Advance.Fallback),
		},
	}
	if plan.Objective != nil {
		line := agentBriefLine(plan.Objective.Sentence)
		out.Objective = &line
		out.Caveat = plan.Objective.Caveat
	}
	if plan.Opening != nil {
		line := agentBriefLine(*plan.Opening)
		out.Opening = &line
	}
	if plan.TopRisk != nil {
		out.TopRisk = &agents.MeetingPlanRiskPart{
			Text:  agentBriefLine(plan.TopRisk.Text),
			Say:   plan.TopRisk.ResponsePlan.Say,
			Show:  plan.TopRisk.ResponsePlan.Show,
			Avoid: plan.TopRisk.ResponsePlan.Avoid,
		}
	}
	for _, ask := range plan.LikelyAsks {
		out.LikelyAsks = append(out.LikelyAsks, agents.MeetingPlanAskPart{
			Question:  ask.Question,
			Basis:     agentBriefLine(ask.Basis),
			Relevance: string(ask.Relevance),
			Prepare:   ask.Prepare,
		})
	}
	for _, question := range plan.Questions {
		out.Questions = append(out.Questions, agents.MeetingPlanAskLine{
			Ask:       question.Ask,
			Why:       question.Why,
			ListenFor: question.ListenFor,
			Evidence:  agentCites(question.Evidence),
		})
	}
	for _, scenario := range plan.Scenarios {
		out.Scenarios = append(out.Scenarios, agents.MeetingPlanBranch{
			Label:    scenario.Label,
			Play:     scenario.Play,
			Evidence: agentCites(scenario.Evidence),
		})
	}
	for _, moment := range plan.AccountArc {
		out.Arc = append(out.Arc, agents.MeetingPlanMoment{
			From:    moment.From.UTC().Format(time.RFC3339),
			To:      moment.To.UTC().Format(time.RFC3339),
			Title:   moment.Title,
			Summary: agentBriefLine(moment.Summary),
		})
	}
	for _, unknown := range plan.Unknowns {
		out.Unknowns = append(out.Unknowns, agents.MeetingPlanGap{
			Kind: string(unknown.Kind), Question: unknown.Question,
		})
	}
	return out
}

func agentCites(evidence []crmcontracts.OrganizationBriefEvidence) []agents.MeetingBriefCite {
	out := make([]agents.MeetingBriefCite, 0, len(evidence))
	for _, cited := range evidence {
		out = append(out, agents.MeetingBriefCite{
			RecordType: string(cited.EntityType),
			RecordID:   ids.UUID(cited.EntityId),
		})
	}
	return out
}
