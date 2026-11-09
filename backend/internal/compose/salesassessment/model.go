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
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PromptVersion changes when the prompt or the reply shape changes, so a
// cached assessment written under an older prompt is not served as current.
const PromptVersion = "2026-11-09.2"

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
	Display        string `json:"display,omitempty"`
	CoverageDetail string `json:"coverage_detail,omitempty"`
	ID             string `json:"id"`
	Label          string `json:"label"`
	Value          string `json:"value"`
	Unit           string `json:"unit"`
	Window         string `json:"window"`
	Coverage       string `json:"coverage"`
}

// CandidateIn is a deal the next decision may name. Display is the amount
// the way the page prints it, as a cell's is.
type CandidateIn struct {
	Display string `json:"display,omitempty"`
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
{"paragraphs":[[{"text":"...","kind":"observation|claim|hypothesis","cells":["<cell id>", ...],"records":["<candidate deal or decision id, such as d1 or r1>", ...]}, ...]],"action_ids":["<supplied action id>"],"next_decision":{"deal_id":"...","owner_id":"...","text":"..."} or null,"confidence":"low|medium|high"}
Each paragraph has at most six sentences of at most 240 characters. Each sentence cites in "cells" the cell ids it rests on, at least one, and only ids from "cells"; a candidate deal or decision ref goes in "records", never in "cells": a sentence citing no cell is dropped, and a dropped sentence refuses the whole reply. A sentence that speaks of a candidate deal or a decision also cites that id in "records", beside the cell it bears on, such as the open pipeline the candidate deals sit in. Connect the reasoning across sentences. "kind" says what the sentence is: "observation" reads a figure as it is; "claim" repeats what a source reported; "hypothesis" is your reading of why, and says so.
Write each quantity as {{cell id}}, for example {{metric:bookings_won}}, and cite that cell. Name a candidate deal by its "name" in plain text; {{deal id}}, such as {{d1}}, stands only for that deal's amount, at most once, in a sentence that names the deal and cites it in "records". Say what a decision decided in plain words, never as {{ }}. The application substitutes its whole-number display. Never write count as a unit, raw basis/status codes, an assurance that you checked something, or a list of clean checks. Do not add, divide or round figures yourself. Literal numbers must match a cited value. A decision or owner ref never appears in text, inside {{ }} or out, and a deal ref appears only as its {{ }} amount.
"next_decision" names the one deal to act on, from "candidate_deals" only, with that deal's "owner_id", and says in one sentence what to decide. Choose a candidate deal no open decision covers. Return null only when there is no candidate deal or every one already has an open decision.
"decisions" are what earlier reviews decided. A done one with a "result" is work that happened; an open one is still owed. Acknowledge them in plain words before you recommend again, never writing a ref or {{ }} in the text. Such a sentence still cites a cell, as in {"text":"You still owe the call to the buyer about the budget owner.","cells":["metric:open_pipeline"],"records":["r1"]}, and never recommend what an open decision already covers.
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
	shown, _ := asShown(in)
	encoded, err := json.Marshal(shown)
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
	Text    string   `json:"text"`
	Kind    string   `json:"kind"`
	Cells   []string `json:"cells"`
	Records []string `json:"records"`
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

var (
	sentenceKinds = map[string]bool{"observation": true, "claim": true, "hypothesis": true}
	confidences   = map[string]bool{"low": true, "medium": true, "high": true}
)

// Parse reads the reply and keeps what the figures carry. A sentence citing
// no known cell, or quoting a number nothing it cites holds, is dropped. A
// next decision naming a deal outside the candidates is dropped. A reply left
// with no sentence is refused, and the caller serves the deterministic floor.
// The reply is read in the refs the request showed; only the next decision
// carries record ids out.
func Parse(reply string, in Input) (Written, error) {
	var parsed replyShape
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return Written{}, fmt.Errorf("parse assessment: %w", err)
	}
	// Reader text may spell neither a record's id nor the ref it was shown by.
	known := knownIDs(in)
	in, refs := asShown(in)
	ev := evidenceOf(in)
	maps.Copy(known, knownIDs(in))
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
			kept, ok, err := keepSentence(line, ev, known)
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
	actionIDs, err := keepActions(parsed.ActionIDs, in.Actions)
	if err != nil {
		return Written{}, err
	}
	out.ActionIDs = actionIDs
	if len(out.Sentences) == 0 {
		return Written{}, errors.New("assessment reply reads no figure")
	}
	if parsed.Next != nil {
		out.Next = refs.recordIDs(keepDecision(parsed.Next.DealID, parsed.Next.OwnerID, parsed.Next.Text, in, ev, known))
	}
	if confidences[parsed.Confidence] {
		out.Confidence = parsed.Confidence
	}
	return out, nil
}

// keepActions keeps the prioritised actions, refusing the reply when it names
// one the request did not supply or more than three.
func keepActions(ids []string, supplied []ActionIn) ([]string, error) {
	var kept []string
	for _, id := range ids {
		if !slices.ContainsFunc(supplied, func(action ActionIn) bool { return action.ID == id }) {
			return nil, errors.New("assessment names an unsupported action")
		}
		if len(kept) == 3 {
			return nil, errors.New("too many assessment actions")
		}
		kept = append(kept, id)
	}
	return kept, nil
}

// keepSentence keeps a sentence whose prose passes, whose citations allow a
// reading, and whose quantities its citations carry. An error refuses the
// whole reply.
func keepSentence(line replySentence, ev evidence, known map[string]bool) (Sentence, bool, error) {
	text := strings.TrimSpace(line.Text)
	if refused, err := refusedProse(text, ev, known); refused || err != nil {
		return Sentence{}, false, err
	}
	kept, cited, usable := citedSources(slices.Concat(line.Cells, line.Records), ev)
	if !usable {
		return Sentence{}, false, nil
	}
	raw, displayed, valid := renderQuantities(text, cited, ev.sources)
	if !valid {
		return Sentence{}, false, nil
	}
	if len([]rune(displayed)) > maxSentenceRune {
		return Sentence{}, false, errors.New("formatted sentence exceeds the assessment bounds")
	}
	if len(kept) == 0 || !numbersCarried(raw, cited, ev.sources) {
		return Sentence{}, false, nil
	}
	return Sentence{Text: displayed, Kind: sentenceKind(line.Kind), Cells: kept}, true, nil
}

// refusedProse says whether a sentence's own words already rule it out. It
// errs when they refuse the whole reply, past the bound or spelling an id.
func refusedProse(text string, ev evidence, known map[string]bool) (bool, error) {
	if machineWording(text) || text == "" {
		return true, nil
	}
	if len([]rune(text)) > maxSentenceRune {
		return true, errors.New("a sentence exceeds the assessment's bounds")
	}
	if err := refuseIDs(factToken.ReplaceAllString(text, ""), known); err != nil {
		return true, err
	}
	return !ev.quantitiesFit(text), nil
}

// citedSources splits what a sentence cites into the cells that ground it and
// every source its figures may come from. It is not usable when a cited cell's
// coverage allows no reading; an unknown citation is skipped.
func citedSources(citations []string, ev evidence) (kept, cited []string, usable bool) {
	for _, id := range citations {
		source, ok := ev.sources[id]
		if !ok {
			continue
		}
		if ev.cells[id] {
			if source.Coverage == "failed" || source.Coverage == "withheld" || source.Coverage == "unsupported" || source.Value == "not measured" {
				return nil, nil, false
			}
			kept = append(kept, id)
		}
		cited = append(cited, id)
	}
	return kept, cited, true
}

// sentenceKind is the kind a sentence declared, or observation when it
// declared none the vocabulary holds.
func sentenceKind(kind string) string {
	if !sentenceKinds[kind] {
		return "observation"
	}
	return kind
}

// keepDecision keeps a decision on a candidate deal, for that deal's owner,
// whose numbers the request carried. A figure the model invented for a deal
// is the one thing the reader would act on without a check.
func keepDecision(dealID, ownerID, text string, in Input, ev evidence, known map[string]bool) *NextDecision {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > maxDecisionRune || refuseIDs(factToken.ReplaceAllString(text, ""), known) != nil {
		return nil
	}
	raw, displayed, valid := renderQuantities(text, ev.order, ev.sources)
	if !valid || !ev.quantitiesFit(text) || machineWording(text) || len([]rune(displayed)) > maxDecisionRune || !numbersCarried(raw, ev.order, ev.sources) {
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
		return &NextDecision{DealID: dealID, OwnerID: ownerID, Text: displayed}
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
		if id != "" && spellsWord(text, id) {
			return errors.New("reply spells an id in reader text")
		}
	}
	return nil
}

// spellsWord reports whether text holds word with no letter or digit glued to
// either side: the ref "r1" is a leak in "see r1." and a name in "Tier1".
func spellsWord(text, word string) bool {
	for at := strings.Index(text, word); at >= 0; {
		before, _ := utf8.DecodeLastRuneInString(text[:at])
		after, _ := utf8.DecodeRuneInString(text[at+len(word):])
		if !isWordRune(before) && !isWordRune(after) {
			return true
		}
		next := strings.Index(text[at+1:], word)
		if next < 0 {
			return false
		}
		at += 1 + next
	}
	return false
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

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
