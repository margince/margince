// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package salesassessment builds the request and reads the reply of the
// sales review's assessment. The reply is a few sentences about a report's
// measured figures and one next decision on a deal the reader may see.
//
// It holds no service and no store. The compose package gathers the cells,
// the candidate deals and the recorded decisions, and the certification case
// calls the same Request and Parse production calls.
package salesassessment

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PromptVersion changes when the prompt or the reply shape changes, so a
// cached assessment written under an older prompt is not served as current.
const PromptVersion = "2026-11-03.3"

// Bounds apply before grounding, so malformed replies cannot grow the reading.
const (
	maxSentences    = 6
	maxSentenceRune = 240
	maxDecisionRune = 240
	maxParagraphs   = 4
)

// Input is what the model reads: measured cells with their units and
// windows, never a record. Candidate deals are the evidence rows the reader
// may see; decisions are the ones recorded from earlier reviews.
type Input struct {
	View       string        `json:"view"`
	Actions    []ActionIn    `json:"actions"`
	Frame      FrameIn       `json:"frame"`
	Cells      []CellIn      `json:"cells"`
	Candidates []CandidateIn `json:"candidate_deals"`
	Decisions  []DecisionIn  `json:"decisions"`
	Earlier    string        `json:"earlier_assessment,omitempty"`
}

// ActionIn is a measured application action the model may prioritise.
type ActionIn struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Cell  string `json:"cell"`
}

// FrameIn names what the report is about.
type FrameIn struct {
	Timezone   string `json:"timezone"`
	Scope      string `json:"scope"`
	Period     string `json:"period"`
	Window     string `json:"window"`
	Currency   string `json:"currency"`
	Today      string `json:"today"`
	Product    string `json:"product,omitempty"`
	Initiative string `json:"initiative,omitempty"`
	Country    string `json:"country,omitempty"`
	Industry   string `json:"industry,omitempty"`
}

// CellIn is one measured figure. Value is the figure as text, which is what
// a sentence may quote.
type CellIn struct {
	CoverageDetail string `json:"coverage_detail,omitempty"`
	ID             string `json:"id"`
	Label          string `json:"label"`
	Value          string `json:"value"`
	Unit           string `json:"unit"`
	Window         string `json:"window"`
	Coverage       string `json:"coverage"`
}

// CandidateIn is a deal the next decision may name.
type CandidateIn struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Amount  string `json:"amount"`
	OwnerID string `json:"owner_id"`
}

// DecisionIn is a decision an earlier review recorded, with its outcome.
type DecisionIn struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Deal     string `json:"deal_id"`
	OwnerID  string `json:"owner_id"`
	ReviewOn string `json:"review_on"`
	Status   string `json:"status"`
	Result   string `json:"result,omitempty"`
}

const system = `Interpret the supplied report for its view: sales is a manager's sales review; prospecting is the reader's own work; team is team prospecting. Explain the main commercial takeaway, relationships between the figures and practical next steps in flowing prose. Write up to four substantial paragraphs, normally two to four sentences each. Use fewer when there is less to explain. Never pad sparse evidence, narrate every tile or add generic reassurance, confidence commentary or internal checking status.
The source context states which period and population each figure covers. Never imply that all-open pipeline is the current period's forecast. Zero means no recorded outcome, not failed capture or no work. Missing facts cannot establish causes, customer intent, employee effort or attendance. A median is not an average. A modelled range does not establish the probability of reaching a target. Do not recommend fixing setup without a supplied action.
Every next action must come from "actions". You may prioritise them through "action_ids" but cannot invent a subset, route, count or action. Do not rank deals solely by stage or amount. Respect the difference between historical outcomes and current work. Only discuss changes when cells carry a comparable baseline.

Return ONLY a JSON object with these keys:
{"paragraphs":[[{"text":"...","kind":"observation|claim|hypothesis","cells":["<cell id>", ...]}, ...]],"action_ids":["<supplied action id>"],"next_decision":{"deal_id":"...","owner_id":"...","text":"..."} or null,"confidence":"low|medium|high"}
Each paragraph has at most six sentences of at most 240 characters. Each sentence cites the cell ids it rests on in "cells". Connect the reasoning across sentences. "kind" says what the sentence is: "observation" reads a figure as it is; "claim" repeats what a source reported; "hypothesis" is your reading of why, and says so.
Every number you write must appear in a cited cell's "value". Never add, divide or round figures into a number no cell carries. Never write an id in "text".
"next_decision" names the one deal to act on, from "candidate_deals" only, with that deal's "owner_id", and says in one sentence what to decide. Return null when no candidate deal supports a decision.
"decisions" are what earlier reviews decided. A done one with a "result" is work that happened; an open one is still owed. Acknowledge them before you recommend again, and never recommend what an open decision already covers.
Coverage "withheld", "failed" or "unsupported" prohibits a conclusion from that cell. Do not turn this into boilerplate caveats. Partial coverage permits only the stated covered population. "not measured" is never zero.
"confidence" is how far the cited figures carry the reading.
`

// Request is the one request production sends for this site.
func Request(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         system + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" + fence.Rule("report figures and recorded decisions"),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encode(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

func encode(in Input) string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// Written is what the model wrote once the grounding filter kept it.
type Written struct {
	Sentences  []Sentence
	Paragraphs [][]Sentence
	ActionIDs  []string
	Next       *NextDecision
	Confidence string
}

// Sentence is one reading and the cells it cites.
type Sentence struct {
	Text  string
	Kind  string
	Cells []string
}

// NextDecision names a candidate deal, its owner and what to decide.
type NextDecision struct {
	DealID  string
	OwnerID string
	Text    string
}

type replySentence struct {
	Text  string   `json:"text"`
	Kind  string   `json:"kind"`
	Cells []string `json:"cells"`
}

type replyShape struct {
	Sentences  []replySentence   `json:"sentences"`
	Paragraphs [][]replySentence `json:"paragraphs"`
	ActionIDs  []string          `json:"action_ids"`
	Next       *struct {
		DealID  string `json:"deal_id"`
		OwnerID string `json:"owner_id"`
		Text    string `json:"text"`
	} `json:"next_decision"`
	Confidence string `json:"confidence"`
}

var sentenceKinds = map[string]bool{"observation": true, "claim": true, "hypothesis": true}
var confidences = map[string]bool{"low": true, "medium": true, "high": true}

// Parse reads the reply and keeps what the figures carry. A sentence citing
// no known cell, or quoting a number no cited cell holds, is dropped. A next
// decision naming a deal outside the candidates is dropped. A reply left with
// no sentence is refused, and the caller serves the deterministic floor.
func Parse(reply string, in Input) (Written, error) {
	var parsed replyShape
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return Written{}, fmt.Errorf("parse assessment: %w", err)
	}
	cells := map[string]CellIn{}
	for _, cell := range in.Cells {
		cells[cell.ID] = cell
	}
	known := knownIDs(in)
	out := Written{Confidence: "low"}
	if len(parsed.Paragraphs) > maxParagraphs {
		return Written{}, errors.New("too many assessment paragraphs")
	}
	paragraphs := parsed.Paragraphs
	if len(paragraphs) == 0 && len(parsed.Sentences) > 0 {
		paragraphs = [][]replySentence{parsed.Sentences}
	}
	for _, paragraph := range paragraphs {
		if len(paragraph) > maxSentences {
			return Written{}, errors.New("too many sentences in an assessment paragraph")
		}
		accepted := []Sentence{}
		for _, line := range paragraph {
			kept, ok, err := keepSentence(line.Text, line.Kind, line.Cells, cells, known)
			if err != nil {
				return Written{}, err
			}
			if !ok {
				if len(parsed.Paragraphs) == 0 {
					continue
				}
				return Written{}, errors.New("assessment paragraph contains an unsupported sentence")
			}
			accepted = append(accepted, kept)
			if len(out.Sentences) < maxSentences {
				out.Sentences = append(out.Sentences, kept)
			}
		}
		if len(accepted) > 0 {
			out.Paragraphs = append(out.Paragraphs, accepted)
		}
	}
	for _, id := range parsed.ActionIDs {
		found := false
		for _, action := range in.Actions {
			if action.ID == id {
				found = true
				break
			}
		}
		if !found {
			return Written{}, errors.New("assessment names an unsupported action")
		}
		if len(out.ActionIDs) == 3 {
			return Written{}, errors.New("too many assessment actions")
		}
		out.ActionIDs = append(out.ActionIDs, id)
	}
	if len(out.Sentences) == 0 {
		return Written{}, errors.New("assessment reply reads no figure")
	}
	if parsed.Next != nil {
		out.Next = keepDecision(parsed.Next.DealID, parsed.Next.OwnerID, parsed.Next.Text, in, known)
	}
	if confidences[parsed.Confidence] {
		out.Confidence = parsed.Confidence
	}
	return out, nil
}

func keepSentence(text, kind string, cited []string, cells map[string]CellIn, known map[string]bool) (Sentence, bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Sentence{}, false, nil
	}
	if len([]rune(text)) > maxSentenceRune {
		return Sentence{}, false, errors.New("a sentence exceeds the assessment's bounds")
	}
	if err := refuseIDs(text, known); err != nil {
		return Sentence{}, false, err
	}
	if !sentenceKinds[kind] {
		kind = "observation"
	}
	var kept []string
	for _, id := range cited {
		if cell, ok := cells[id]; ok {
			if cell.Coverage == "failed" || cell.Coverage == "withheld" || cell.Coverage == "unsupported" || cell.Value == "not measured" {
				return Sentence{}, false, nil
			}
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 || !numbersCarried(text, kept, cells) {
		return Sentence{}, false, nil
	}
	return Sentence{Text: text, Kind: kind, Cells: kept}, true, nil
}

// keepDecision keeps a decision on a candidate deal, for that deal's owner,
// whose numbers the cells carry. A figure the model invented for a deal is
// the one thing the reader would act on without a check.
func keepDecision(dealID, ownerID, text string, in Input, known map[string]bool) *NextDecision {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > maxDecisionRune || refuseIDs(text, known) != nil {
		return nil
	}
	cells := map[string]CellIn{}
	var every []string
	for _, cell := range in.Cells {
		cells[cell.ID] = cell
		every = append(every, cell.ID)
	}
	if !numbersCarried(text, every, cells) {
		return nil
	}
	for _, candidate := range in.Candidates {
		if candidate.ID != dealID {
			continue
		}
		if ownerID == "" {
			ownerID = candidate.OwnerID
		}
		if ownerID != candidate.OwnerID {
			return nil
		}
		return &NextDecision{DealID: dealID, OwnerID: ownerID, Text: text}
	}
	return nil
}

// refuseIDs refuses an id in reader text: the shared rule for a uuid in prose,
// and the ids this request carried in any spelling.
func refuseIDs(text string, known map[string]bool) error {
	if claims.SpellsRecordID(text) {
		return errors.New("reply spells a record id in reader text")
	}
	for id := range known {
		if id != "" && strings.Contains(text, id) {
			return errors.New("reply spells an id in reader text")
		}
	}
	return nil
}

// knownIDs collects the ids the input carried; reader text must not spell one.
func knownIDs(in Input) map[string]bool {
	known := map[string]bool{}
	for _, cell := range in.Cells {
		known[cell.ID] = true
	}
	for _, candidate := range in.Candidates {
		known[candidate.ID] = true
		known[candidate.OwnerID] = true
	}
	for _, decision := range in.Decisions {
		known[decision.ID] = true
	}
	return known
}
