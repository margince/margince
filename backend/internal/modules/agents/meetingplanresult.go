// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The preparation plan, in the tool surface's vocabulary.
//
// It crosses the seam for the same reason the sections do: an agent asked to
// prepare its principal for a meeting needs what to DO, and a plan flattened to
// prose on the way through is a plan it cannot act on. Every cited field keeps
// its evidence, so an agent can open what a claim rests on rather than repeat
// it on trust.

// MeetingPlanResult is the plan as an agent reads it.
type MeetingPlanResult struct {
	Readiness  string                 `json:"readiness"`
	Type       string                 `json:"meeting_type"`
	Confidence string                 `json:"meeting_type_confidence"`
	Objective  *MeetingBriefLine      `json:"objective,omitempty"`
	Caveat     string                 `json:"objective_caveat,omitempty"`
	Opening    *MeetingBriefLine      `json:"opening,omitempty"`
	TopRisk    *MeetingPlanRiskPart   `json:"top_risk,omitempty"`
	LikelyAsks []MeetingPlanAskPart   `json:"likely_asks,omitempty"`
	Questions  []MeetingPlanAskLine   `json:"questions,omitempty"`
	Scenarios  []MeetingPlanBranch    `json:"scenarios,omitempty"`
	Arc        []MeetingPlanMoment    `json:"account_arc,omitempty"`
	Advance    MeetingPlanAdvancePart `json:"advance"`
	Unknowns   []MeetingPlanGap       `json:"unknowns,omitempty"`
}

// MeetingPlanRiskPart is the watch-out and what to do about it.
type MeetingPlanRiskPart struct {
	Text  MeetingBriefLine `json:"text"`
	Say   string           `json:"say"`
	Show  string           `json:"show"`
	Avoid string           `json:"avoid"`
}

// MeetingPlanAskPart is something they are likely to ask us.
type MeetingPlanAskPart struct {
	Question  string           `json:"question"`
	Basis     MeetingBriefLine `json:"basis"`
	Relevance string           `json:"relevance"`
	Prepare   string           `json:"prepare"`
}

// MeetingPlanAskLine is something to ask them.
type MeetingPlanAskLine struct {
	Ask       string             `json:"ask"`
	Why       string             `json:"why"`
	ListenFor string             `json:"listen_for"`
	Evidence  []MeetingBriefCite `json:"evidence"`
}

// MeetingPlanBranch is what the meeting may turn into.
type MeetingPlanBranch struct {
	Label    string             `json:"label"`
	Play     string             `json:"play"`
	Evidence []MeetingBriefCite `json:"evidence"`
}

// MeetingPlanMoment is one stretch of the account's history.
type MeetingPlanMoment struct {
	From    string           `json:"from"`
	To      string           `json:"to"`
	Title   string           `json:"title,omitempty"`
	Summary MeetingBriefLine `json:"summary"`
}

// MeetingPlanAdvancePart is the three ways the meeting can end well.
type MeetingPlanAdvancePart struct {
	Minimum  MeetingBriefLine `json:"minimum"`
	Best     MeetingBriefLine `json:"best"`
	Fallback MeetingBriefLine `json:"fallback"`
}

// MeetingPlanGap is what the record does not say.
type MeetingPlanGap struct {
	Kind     string `json:"kind"`
	Question string `json:"question"`
}

// Citations are every record the plan points at.
//
// Held by: TestCitationsReachesEveryFieldThatCanCarryEvidence
// (backend/internal/modules/agents/meetingplanresult_test.go)
//
// The read bound charges what a tool call REACHED, and the plan reaches records
// the sections do not — the arc walks a year of history the eight sections
// never mention. Charging the sections alone would make the richest read the
// cheapest one, which is the wrong way round for a budget that exists to bound
// how much of a workspace one call can pull.
func (p *MeetingPlanResult) Citations() []MeetingBriefCite {
	if p == nil {
		return nil
	}
	var cites []MeetingBriefCite
	add := func(line MeetingBriefLine) { cites = append(cites, line.Evidence...) }
	if p.Objective != nil {
		add(*p.Objective)
	}
	if p.Opening != nil {
		add(*p.Opening)
	}
	if p.TopRisk != nil {
		add(p.TopRisk.Text)
	}
	for _, ask := range p.LikelyAsks {
		add(ask.Basis)
	}
	for _, question := range p.Questions {
		cites = append(cites, question.Evidence...)
	}
	for _, scenario := range p.Scenarios {
		cites = append(cites, scenario.Evidence...)
	}
	for _, moment := range p.Arc {
		add(moment.Summary)
	}
	add(p.Advance.Minimum)
	add(p.Advance.Best)
	add(p.Advance.Fallback)
	return cites
}
