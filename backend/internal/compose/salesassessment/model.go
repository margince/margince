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
	"unicode"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PromptVersion changes when the prompt or the reply shape changes, so a
// cached assessment written under an older prompt is not served as current.
const PromptVersion = "2026-11-03.1"

// The reply's bounds: the panel sits beside one chart, not under it.
const (
	maxSentences    = 6
	maxSentenceRune = 240
	maxDecisionRune = 240
)

// Input is what the model reads: measured cells with their units and
// windows, never a record. Candidate deals are the evidence rows the reader
// may see; decisions are the ones recorded from earlier reviews.
type Input struct {
	Frame      FrameIn       `json:"frame"`
	Cells      []CellIn      `json:"cells"`
	Candidates []CandidateIn `json:"candidate_deals"`
	Decisions  []DecisionIn  `json:"decisions"`
	Earlier    string        `json:"earlier_assessment,omitempty"`
}

// FrameIn names what the report is about.
type FrameIn struct {
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
	ID       string `json:"id"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Unit     string `json:"unit"`
	Window   string `json:"window"`
	Coverage string `json:"coverage"`
}

// CandidateIn is a deal the next decision may name.
type CandidateIn struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Amount  string `json:"amount"`
	Stage   string `json:"stage,omitempty"`
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

const system = `You assess one sales report for the manager reading it, from a JSON summary of its measured figures, the open deals behind them and the decisions recorded at earlier reviews.
Return ONLY a JSON object with these keys:
{"sentences":[{"text":"...","kind":"observation|claim|hypothesis","cells":["<cell id>", ...]}, ...],"next_decision":{"deal_id":"...","owner_id":"...","text":"..."} or null,"confidence":"low|medium|high"}
"sentences" is two to six sentences. Each states one thing a figure shows and cites the cell ids it rests on in "cells". "kind" says what the sentence is: "observation" reads a figure as it is; "claim" repeats what a source reported; "hypothesis" is your reading of why, and says so.
Every number you write must appear in a cited cell's "value". Never add, divide or round figures into a number no cell carries. Never write an id in "text".
"next_decision" names the one deal to act on, from "candidate_deals" only, with that deal's "owner_id", and says in one sentence what to decide. Return null when no candidate deal supports a decision.
"decisions" are what earlier reviews decided. A done one with a "result" is work that happened; an open one is still owed. Acknowledge them before you recommend again, and never recommend what an open decision already covers.
Coverage "withheld" or "failed" on a cell means its figure is not whole: say so rather than reading it as a total.
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

type replyShape struct {
	Sentences []struct {
		Text  string   `json:"text"`
		Kind  string   `json:"kind"`
		Cells []string `json:"cells"`
	} `json:"sentences"`
	Next *struct {
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
	for _, line := range parsed.Sentences {
		kept, ok, err := keepSentence(line.Text, line.Kind, line.Cells, cells, known)
		if err != nil {
			return Written{}, err
		}
		if ok {
			out.Sentences = append(out.Sentences, kept)
		}
		if len(out.Sentences) == maxSentences {
			break
		}
	}
	if len(out.Sentences) == 0 {
		return Written{}, errors.New("assessment reply reads no figure")
	}
	if parsed.Next != nil {
		out.Next = keepDecision(parsed.Next.DealID, parsed.Next.OwnerID, parsed.Next.Text, in.Candidates, known)
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
		if _, ok := cells[id]; ok {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 || !numbersCarried(text, kept, cells) {
		return Sentence{}, false, nil
	}
	return Sentence{Text: text, Kind: kind, Cells: kept}, true, nil
}

// numbersCarried says every number in the text appears in a cited cell's
// value. Digits are compared without separators, so "21.6k" and "€21,600"
// carry the same figure.
func numbersCarried(text string, cited []string, cells map[string]CellIn) bool {
	carried := map[string]bool{}
	for _, id := range cited {
		for _, run := range digitRuns(cells[id].Value) {
			carried[run] = true
		}
	}
	for _, run := range digitRuns(text) {
		if len(run) < 2 {
			continue
		}
		if !carried[run] && !carriedWithin(run, carried) {
			return false
		}
	}
	return true
}

// carriedWithin admits a shorter spelling of a carried figure, as "21.6" of
// "21600" is, and a year that opens a cited window.
func carriedWithin(run string, carried map[string]bool) bool {
	for value := range carried {
		if strings.HasPrefix(value, run) {
			return true
		}
	}
	return false
}

// digitRuns is each run of digits in the text, with separators inside a
// number removed, so "1,200.5" reads as "12005" wherever it is spelled.
func digitRuns(text string) []string {
	var runs []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			runs = append(runs, current.String())
			current.Reset()
		}
	}
	for i, r := range text {
		switch {
		case unicode.IsDigit(r):
			current.WriteRune(r)
		case (r == '.' || r == ',') && current.Len() > 0 && i+1 < len(text) && unicode.IsDigit(rune(text[i+1])):
		default:
			flush()
		}
	}
	flush()
	return runs
}

func keepDecision(dealID, ownerID, text string, candidates []CandidateIn, known map[string]bool) *NextDecision {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > maxDecisionRune || refuseIDs(text, known) != nil {
		return nil
	}
	for _, candidate := range candidates {
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

func refuseIDs(text string, known map[string]bool) error {
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
