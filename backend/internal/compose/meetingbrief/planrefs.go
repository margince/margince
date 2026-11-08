// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"slices"

	"github.com/margince/margince/backend/internal/compose/claims"
)

// planRefs names each record id the plan prompt shows by a ref, and reads a
// reply's citations back. PlanRequest and ParsePlan both call it, so a ref
// names one record in both.
func planRefs(in Input, floor Plan) claims.Refs {
	return claims.MintRefs(encodePlanPrompt(planPromptOf(in, floor)), knownRecords(in))
}

// readPlanRefs reads every citation of a reply back to the record id the
// request showed it by; a ref it never showed reads as no record, which no
// grounding accepts. An item whose prose writes a ref is dropped, as one that
// spells an id is.
func readPlanRefs(reply planReply, shown claims.Refs) planReply {
	names := func(lines ...string) bool { return slices.ContainsFunc(lines, shown.NamedIn) }
	read := func(evidence []replyEvidence) []replyEvidence {
		out := make([]replyEvidence, 0, len(evidence))
		for _, cited := range evidence {
			cited.EntityID, _ = shown.ID(cited.EntityType, cited.EntityID)
			out = append(out, cited)
		}
		return out
	}
	reply.Objective = readSentence(reply.Objective, names, read)
	reply.Opening = readSentence(reply.Opening, names, read)
	if risk := reply.TopRisk; risk != nil {
		reply.TopRisk = nil
		if !names(risk.Text, risk.Say, risk.Show, risk.Avoid) {
			risk.Evidence = read(risk.Evidence)
			reply.TopRisk = risk
		}
	}
	reply.LikelyAsks = readItems(reply.LikelyAsks, func(ask replyAsk) (replyAsk, bool) {
		ask.Evidence = read(ask.Evidence)
		return ask, !names(ask.Question, ask.Basis, ask.Prepare)
	})
	reply.Questions = readItems(reply.Questions, func(question replyQuestion) (replyQuestion, bool) {
		question.Evidence = read(question.Evidence)
		return question, !names(question.Ask, question.Why, question.ListenFor)
	})
	reply.Scenarios = readItems(reply.Scenarios, func(scenario replyScenario) (replyScenario, bool) {
		scenario.Evidence = read(scenario.Evidence)
		return scenario, !names(scenario.Label, scenario.Play)
	})
	return reply
}

func readSentence(
	sentence *replySentence, names func(...string) bool, read func([]replyEvidence) []replyEvidence,
) *replySentence {
	if sentence == nil || names(sentence.Text) {
		return nil
	}
	copied := *sentence
	copied.Evidence = read(copied.Evidence)
	return &copied
}

// readItems keeps each item read reports kept, as read rewrote it.
func readItems[T any](items []T, read func(T) (T, bool)) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		if kept, ok := read(item); ok {
			out = append(out, kept)
		}
	}
	return out
}
