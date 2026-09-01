// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The preparation plan: what to DO in the room.
//
// `sections` says what is known; this says what to do about it. The two are
// built from the same Input and the same ranked claims, so they cannot disagree
// about which promise is the sharpest one outstanding — the sections name it,
// and the plan aims the meeting at closing it.
//
// Everything here is deterministic. A model can rewrite it into better prose
// (planwrite.go) and can never widen what it covers, exactly as the sections
// work: the floor is what a deployment with no model still gets, and it has to
// be worth reading on its own.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Plan is the internal shape, before grounding and wire rendering.
type Plan struct {
	Type       MeetingType
	Objective  *Objective
	Opening    *Sentence
	TopRisk    *Risk
	LikelyAsks []Ask
	Questions  []Question
	Scenarios  []Scenario
	Arc        []ArcSentence
	Advance    Advance
	Unknowns   []Unknown
}

// Objective is the outcome to earn plus the reminder not to force it.
type Objective struct {
	Sentence Sentence
	Caveat   string
}

// Risk is the one thing that can change the conversation, and the response.
type Risk struct {
	Text     Sentence
	Response Response
}

// Response is what to say, show and not promise.
type Response struct {
	Say   string
	Show  string
	Avoid string
}

// Ask is a question they are likely to put to us.
type Ask struct {
	Question  string
	Basis     Sentence
	Relevance crmcontracts.MeetingPlanTier
	Prepare   string
}

// Question is one we should put to them.
type Question struct {
	Ask       string
	Why       string
	ListenFor string
	Evidence  []Evidence
}

// Scenario is what the meeting may turn into.
type Scenario struct {
	Label    string
	Play     string
	Evidence []Evidence
}

// ArcSentence is one moment of the account arc, rendered.
type ArcSentence struct {
	Moment  ArcMoment
	Summary Sentence
}

// Advance is the three ways this meeting can end well.
type Advance struct {
	Minimum  Sentence
	Best     Sentence
	Fallback Sentence
}

// Unknown is a gap in the record and the question that closes it.
type Unknown struct {
	Kind     crmcontracts.MeetingPlanUnknownKind
	Question string
}

// DeterministicPlan builds the floor.
//
// It shares ONE ranked claim set with the sections, passed in rather than
// re-derived: two rankings of the same claims would drift, and the drift would
// show as a brief whose goal names one promise and whose plan aims at another.
func DeterministicPlan(in Input, ranked *rankedClaims) Plan {
	typ := classifyMeeting(in)
	arc := accountArc(in)
	unknowns := unknownsFor(in, typ, arc)
	plan := Plan{
		Type:      typ,
		Objective: objectiveFor(in, typ, ranked),
		Opening:   openingFor(in, typ),
		TopRisk:   topRiskFor(in, ranked),
		Arc:       arcSentences(arc),
		Advance:   advanceFor(in, typ),
		Scenarios: scenariosFor(in, typ),
		Unknowns:  unknowns,
	}
	plan.LikelyAsks = likelyAsksFor(in, ranked)
	plan.Questions = questionsFor(in, unknowns, ranked)
	return plan
}

// arcSentences renders each moment as a cited fact.
func arcSentences(moments []ArcMoment) []ArcSentence {
	out := make([]ArcSentence, 0, len(moments))
	for _, moment := range moments {
		ids := readableIDs(moment)
		if len(ids) == 0 {
			continue
		}
		evidence := make([]Evidence, 0, len(ids))
		for _, id := range ids {
			evidence = append(evidence, Evidence{EntityType: citeActivity, EntityID: id})
		}
		out = append(out, ArcSentence{
			Moment: moment,
			Summary: Sentence{
				Text:     arcSummary(moment, conversationCount(moment)),
				Evidence: evidence,
			},
		})
	}
	return out
}

// conversationCount is how many CONVERSATIONS a moment holds, which is what
// the folding was for: a twelve-message reply chain is one conversation, and
// reporting it as twelve would undo the fold in the sentence a reader sees.
func conversationCount(moment ArcMoment) int {
	return len(moment.Threads)
}
