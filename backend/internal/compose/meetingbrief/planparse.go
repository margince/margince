// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Reading the model's plan back, and refusing what it may not say.
//
// Three refusals, each closing a different hole. A field whose nature is not
// one the field allows is dropped, so a judgement cannot be filed where the
// plan states facts. A sentence citing a record the briefing never carried is
// dropped, which is the same allowlist the sections run. And `unknowns` is not
// read from the reply at all: a gap is a fact about the RECORD, and a model
// inventing one would be inventing the absence of evidence.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// scenarioCap bounds the branches. Three is a thing a rep can hold walking in;
// a decision tree is triage handed back to them.
const scenarioCap = 3

// groundedLine is claims.Grounded over this package's sentence shape.
func groundedLine(sentence Sentence, known map[Evidence]bool) bool {
	return claims.Grounded(sentence, known)
}

// groundedEvidenceOnly is the same rule for a field that carries citations
// without prose of its own: at least one, and every one a record the reader
// can open.
func groundedEvidenceOnly(cited []Evidence, known map[Evidence]bool) bool {
	if len(cited) == 0 {
		return false
	}
	for _, one := range cited {
		if !known[Evidence{EntityType: one.EntityType, EntityID: one.EntityID}] {
			return false
		}
	}
	return true
}

// planReply is the shape the prompt asks for.
type planReply struct {
	Objective  *replySentence  `json:"objective"`
	Opening    *replySentence  `json:"opening"`
	TopRisk    *replyRisk      `json:"top_risk"`
	LikelyAsks []replyAsk      `json:"likely_asks"`
	Questions  []replyQuestion `json:"questions"`
	Scenarios  []replyScenario `json:"scenarios"`
}

type replyEvidence struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

type replySentence struct {
	Text     string          `json:"text"`
	Nature   string          `json:"nature"`
	Evidence []replyEvidence `json:"evidence"`
}

type replyRisk struct {
	Text     string          `json:"text"`
	Evidence []replyEvidence `json:"evidence"`
	Say      string          `json:"say"`
	Show     string          `json:"show"`
	Avoid    string          `json:"avoid"`
}

type replyAsk struct {
	Question  string          `json:"question"`
	Basis     string          `json:"basis"`
	Evidence  []replyEvidence `json:"evidence"`
	Relevance string          `json:"relevance"`
	Prepare   string          `json:"prepare"`
}

type replyQuestion struct {
	Ask       string          `json:"ask"`
	Why       string          `json:"why"`
	ListenFor string          `json:"listen_for"`
	Evidence  []replyEvidence `json:"evidence"`
}

type replyScenario struct {
	Label    string          `json:"label"`
	Play     string          `json:"play"`
	Evidence []replyEvidence `json:"evidence"`
}

// ParsePlan reads a reply and keeps what it is allowed to say.
//
// Exported alongside PlanRequest so a certification case parses the reply
// production parses.
func ParsePlan(reply string, in Input, floor Plan) (Plan, error) {
	var parsed planReply
	if err := json.Unmarshal([]byte(strings.TrimSpace(reply)), &parsed); err != nil {
		return Plan{}, fmt.Errorf("the meeting plan reply is not the JSON the prompt asked for: %w", err)
	}
	known := knownRecords(in)
	// Type and unknowns are the floor's, always. The first is read from
	// signals the records carry and the second from their absence; neither is
	// a thing a writer may propose.
	written := Plan{Type: floor.Type, Unknowns: floor.Unknowns}

	if sentence, ok := keptPlanSentence(parsed.Objective, "objective", known); ok {
		written.Objective = &Objective{Sentence: sentence, Caveat: caveatFor(floor.Type)}
	}
	if sentence, ok := keptPlanSentence(parsed.Opening, "opening", known); ok {
		written.Opening = &sentence
	}
	written.TopRisk = keptRisk(parsed.TopRisk, known)
	written.LikelyAsks = keptAsks(parsed.LikelyAsks, known)
	written.Questions = keptQuestions(parsed.Questions, known)
	written.Scenarios = keptScenarios(parsed.Scenarios, known)

	if emptyReply(written) {
		return Plan{}, fmt.Errorf("the meeting plan reply said nothing this meeting's records support")
	}
	return written, nil
}

// emptyReply reports whether anything the model wrote survived. Everything
// dropped is the same as no answer, and the floor is the honest response.
func emptyReply(written Plan) bool {
	return written.Objective == nil && written.Opening == nil && written.TopRisk == nil &&
		len(written.LikelyAsks) == 0 && len(written.Questions) == 0 && len(written.Scenarios) == 0
}

func keptPlanSentence(
	raw *replySentence, field string, known map[Evidence]bool,
) (Sentence, bool) {
	if raw == nil {
		return Sentence{}, false
	}
	return keptLine(raw.Text, raw.Nature, raw.Evidence, field, known)
}

// keptLine is the one filter every prose field goes through.
func keptLine(
	text, nature string, evidence []replyEvidence, field string, known map[Evidence]bool,
) (Sentence, bool) {
	if strings.TrimSpace(text) == "" {
		return Sentence{}, false
	}
	if nature == "" {
		nature = planNatureDefault[field]
	}
	if !planNatureAllowed[field][nature] {
		// A judgement filed where the field states facts, or a move where it
		// states a reading. Dropped rather than relabelled: relabelling would
		// keep a claim the writer meant differently.
		return Sentence{}, false
	}
	sentence := Sentence{Text: text, Nature: nature, Evidence: citedRecords(evidence)}
	if !groundedLine(sentence, known) {
		return Sentence{}, false
	}
	return sentence, true
}

func citedRecords(evidence []replyEvidence) []Evidence {
	out := make([]Evidence, 0, len(evidence))
	for _, cited := range evidence {
		out = append(out, Evidence{EntityType: cited.EntityType, EntityID: cited.EntityID})
	}
	return out
}

func keptRisk(raw *replyRisk, known map[Evidence]bool) *Risk {
	if raw == nil {
		return nil
	}
	sentence, ok := keptLine(raw.Text, natureAssessment, raw.Evidence, "top_risk", known)
	if !ok {
		return nil
	}
	response := Response{Say: raw.Say, Show: raw.Show, Avoid: raw.Avoid}
	if response.Say == "" || response.Show == "" || response.Avoid == "" {
		// The contract requires all three. A risk with two thirds of a response
		// is a warning with no plan, which is what the reader already had.
		return nil
	}
	return &Risk{Text: sentence, Response: response}
}

func keptAsks(raw []replyAsk, known map[Evidence]bool) []Ask {
	out := make([]Ask, 0, len(raw))
	for _, ask := range raw {
		if len(out) == askCap {
			break
		}
		basis, ok := keptLine(ask.Basis, natureAssessment, ask.Evidence, "likely_asks", known)
		if !ok || strings.TrimSpace(ask.Question) == "" || strings.TrimSpace(ask.Prepare) == "" {
			continue
		}
		out = append(out, Ask{
			Question:  ask.Question,
			Basis:     basis,
			Relevance: tierOf(ask.Relevance),
			Prepare:   ask.Prepare,
		})
	}
	return out
}

// tierOf reads the model's ranking, defaulting to the middle rather than the
// top: a hypothesis nobody ranked is not thereby the most likely one.
func tierOf(raw string) crmcontracts.MeetingPlanTier {
	switch crmcontracts.MeetingPlanTier(strings.ToLower(strings.TrimSpace(raw))) {
	case crmcontracts.MeetingPlanTierHigh:
		return crmcontracts.MeetingPlanTierHigh
	case crmcontracts.MeetingPlanTierLow:
		return crmcontracts.MeetingPlanTierLow
	default:
		return crmcontracts.MeetingPlanTierMedium
	}
}

func keptQuestions(raw []replyQuestion, known map[Evidence]bool) []Question {
	out := make([]Question, 0, len(raw))
	for _, question := range raw {
		if len(out) == questionCap {
			break
		}
		if strings.TrimSpace(question.Ask) == "" || strings.TrimSpace(question.ListenFor) == "" {
			continue
		}
		cited := citedRecords(question.Evidence)
		if !groundedEvidenceOnly(cited, known) {
			continue
		}
		out = append(out, Question{
			Ask: question.Ask, Why: question.Why,
			ListenFor: question.ListenFor, Evidence: cited,
		})
	}
	return out
}

func keptScenarios(raw []replyScenario, known map[Evidence]bool) []Scenario {
	out := make([]Scenario, 0, len(raw))
	for _, scenario := range raw {
		if len(out) == scenarioCap {
			break
		}
		if strings.TrimSpace(scenario.Label) == "" || strings.TrimSpace(scenario.Play) == "" {
			continue
		}
		cited := citedRecords(scenario.Evidence)
		if !groundedEvidenceOnly(cited, known) {
			continue
		}
		out = append(out, Scenario{Label: scenario.Label, Play: scenario.Play, Evidence: cited})
	}
	return out
}
