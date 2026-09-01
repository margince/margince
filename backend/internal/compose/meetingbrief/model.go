// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The model lane: the same facts, written the way a colleague would say them.
//
// The deterministic sections are the floor and stay the floor. They are
// correct and they are dense — nine headings of clipped, comma-joined lines a
// reader has to parse. This lane rewrites those same facts as Margince speaks:
// short sentences, one idea each, the result first. It never adds a fact. Every
// sentence it keeps still cites a record the reader can open, and a sentence
// citing anything the input did not carry is dropped rather than shown.
//
// No lane configured, a lane over budget, or a reply the filter empties, and
// the reader gets the deterministic brief. `generated_by` says which of the two
// they are reading, so a rep can always tell.
//
// The language is the READER's. A brief about a German conversation, read by a
// German rep, that answers in English is a translation task handed back to the
// person who asked for a summary.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the summarize lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// briefSystem is this site's prompt.
//
// The voice rules are Margince's own, stated as rules rather than as a
// description of a personality: a calm colleague leads with the result, writes
// one idea per sentence, and never performs enthusiasm. The banned openers are
// the ones that make a brief read as generated.
const briefSystem = `You write a pre-meeting brief for a salesperson, from a JSON summary of one meeting in their CRM.
Return ONLY a JSON object: {"sections":[{"kind":"header|goal|what_changed|attendees|risks|commitments|deal_state|talking_points|company_context","sentences":[{"text":"...","nature":"fact|assessment|recommendation","evidence":[{"entity_type":"activity|deal|person","entity_id":"..."}]}]}]}.
Write every sentence from the summary and from nothing else. Never invent a fact, a name, a date or a number. If the summary does not say it, do not write it.
Label every sentence. A FACT restates what the summary says. An ASSESSMENT is a reading you draw from it — allowed only in risks and deal_state. A RECOMMENDATION is one concrete move — allowed only in goal and talking_points, at most three in the whole brief.
Cite the ids the summary gave you, in evidence only. An id must never appear in the text a reader sees.
Never open with "Absolutely", "Great question", "I'd be happy to", "Based on the provided context", or any greeting. No exclamation marks. No praise. No summary of what the reader already knows.
Say plainly when something is uncertain or missing rather than filling the gap. If a section has nothing real to say, omit the section.
`

// briefSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
// The brief is written in the installation's language, the same as every other
// AI surface. It used to point the model at a "language" field in the summary
// instead — which json.Marshal emitted as "Language", so the field the prompt
// named was never actually present.
func briefSystemFor(fence promptfence.Fence, lang string) string {
	return briefSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("meeting summary")
}

// maxRecommendations bounds the advice. Three moves are a plan a rep can hold
// walking into a room; a longer list is triage work handed back to them.
const maxRecommendations = 3

// natureAllowed says which kinds of claim each section may carry. Facts are
// welcome anywhere; a reading belongs where the brief is meant to read, and a
// move belongs under a heading that promises one.
var natureAllowed = map[crmcontracts.MeetingBriefSectionKind]map[string]bool{
	crmcontracts.MeetingBriefSectionKindHeader:         {natureFact: true},
	crmcontracts.MeetingBriefSectionKindGoal:           {natureFact: true, natureRecommendation: true},
	crmcontracts.MeetingBriefSectionKindWhatChanged:    {natureFact: true},
	crmcontracts.MeetingBriefSectionKindAttendees:      {natureFact: true},
	crmcontracts.MeetingBriefSectionKindRisks:          {natureFact: true, natureAssessment: true},
	crmcontracts.MeetingBriefSectionKindCommitments:    {natureFact: true},
	crmcontracts.MeetingBriefSectionKindDealState:      {natureFact: true, natureAssessment: true},
	crmcontracts.MeetingBriefSectionKindTalkingPoints:  {natureFact: true, natureRecommendation: true},
	crmcontracts.MeetingBriefSectionKindCompanyContext: {natureFact: true},
}

// natureFact is the contract's default: an unlabelled claim is read as a fact,
// which is the strictest reading — it must be grounded and it may not judge.
const natureFact = string(crmcontracts.Fact)

// Write produces the brief's sections. lane may be nil, which is not an error:
// it is the deployment saying this role runs no model, and the deterministic
// floor is the answer.
func Write(ctx context.Context, lane Completer, in Input, lang string) ([]Section, crmcontracts.WrittenBy) {
	floor := Deterministic(in)
	if lane == nil {
		return floor, crmcontracts.Deterministic
	}
	written, err := writeWithModel(ctx, lane, in, lang)
	if err != nil {
		// The declared degrade posture, not a swallowed error. A model that is
		// unavailable, over budget or answering unparseable JSON must not take
		// the brief down with it: the reader gets the floor, and generated_by
		// tells them which of the two they are reading.
		return floor, crmcontracts.Deterministic
	}
	// A rewrite that dropped a section the floor had is not a rewrite, it is a
	// shorter brief: a model returning one harmless sentence would otherwise
	// take a revised offer or an open risk off the page and look like it
	// worked. Anything the reply left out is restored from the floor, so the
	// model can change how the brief READS and never what it COVERS.
	return withFloorCoverage(written, floor), crmcontracts.Model
}

// withFloorCoverage puts back any section the model did not answer.
func withFloorCoverage(written, floor []Section) []Section {
	answered := map[crmcontracts.MeetingBriefSectionKind]bool{}
	for _, section := range written {
		answered[section.Kind] = true
	}
	merged := make([]Section, 0, len(floor)+len(written))
	for _, kind := range specSequence {
		switch {
		case answered[kind]:
			merged = append(merged, sectionOfKind(written, kind))
		default:
			if from, ok := floorSection(floor, kind); ok {
				merged = append(merged, from)
			}
		}
	}
	return merged
}

func sectionOfKind(sections []Section, kind crmcontracts.MeetingBriefSectionKind) Section {
	for _, section := range sections {
		if section.Kind == kind {
			return section
		}
	}
	return Section{Kind: kind}
}

func floorSection(floor []Section, kind crmcontracts.MeetingBriefSectionKind) (Section, bool) {
	for _, section := range floor {
		if section.Kind == kind && len(section.Sentences) > 0 {
			return section, true
		}
	}
	return Section{}, false
}

func writeWithModel(ctx context.Context, lane Completer, in Input, lang string) ([]Section, error) {
	resp, err := lane.Complete(ctx, BriefRequest(in, lang))
	if err != nil {
		return nil, err
	}
	kept, err := ParseBriefSections(resp.Text, in)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		return nil, errors.New("the meeting brief reply cited nothing in the meeting")
	}
	return kept, nil
}

// BriefRequest builds the one request this site sends. Exported because the
// certification case must issue the SAME request production does — a case that
// rebuilt it would measure a copy, and a copy stays green through the change
// that breaks the original.
//
// The summary carries meeting subjects, contact names and quoted commitments —
// text written by people outside this workspace. It is fenced with a nonce the
// writer has never seen, so no subject line can close the span and be read as
// instruction.
func BriefRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         briefSystemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encodeInput(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

// encodeInput renders the assembled meeting as the JSON the prompt reads.
//
// A summary that cannot be encoded is a programming error, not a runtime one:
// every field is a plain value this package built. The error is still returned
// as text rather than dropped, so a malformed request fails the model call
// loudly instead of asking the model to summarize the empty string.
func encodeInput(in Input) string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// wireReply is the shape the prompt asks for.
type wireReply struct {
	Sections []struct {
		Kind      string `json:"kind"`
		Sentences []struct {
			Text     string `json:"text"`
			Nature   string `json:"nature"`
			Evidence []struct {
				EntityType string `json:"entity_type"`
				EntityID   string `json:"entity_id"`
			} `json:"evidence"`
		} `json:"sentences"`
	} `json:"sections"`
}

// ParseBriefSections reads a sectioned reply, keeping only what is grounded and
// permitted. Exported for the same reason as BriefRequest: the certification
// case must run the filter production runs.
//
// Everything it drops, it drops for one of three reasons — an unknown section,
// a nature that section may not carry, or a citation pointing at a record this
// input never held. A dropped sentence is silent by design: the ones that
// remain still say true things, and a brief explaining its own filtering to a
// rep before a meeting is noise.
func ParseBriefSections(reply string, in Input) ([]Section, error) {
	var parsed wireReply
	if err := json.Unmarshal([]byte(reply), &parsed); err != nil {
		return nil, fmt.Errorf("parse meeting brief sections: %w", err)
	}
	known := knownRecords(in)
	byKind := map[crmcontracts.MeetingBriefSectionKind][]Sentence{}
	// The quota is on the BRIEF, not on one section: a model returning two
	// talking_points sections would otherwise be handed the allowance twice.
	recommendations := 0
	for _, section := range parsed.Sections {
		kind := crmcontracts.MeetingBriefSectionKind(section.Kind)
		allowed, ok := natureAllowed[kind]
		if !ok {
			continue
		}
		for _, raw := range section.Sentences {
			sentence, keep := keptSentence(raw, allowed, known, &recommendations)
			if !keep {
				continue
			}
			byKind[kind] = append(byKind[kind], sentence)
		}
	}
	return orderedSections(byKind), nil
}

// keptSentence decides one sentence and reports whether it survived.
func keptSentence(
	raw struct {
		Text     string `json:"text"`
		Nature   string `json:"nature"`
		Evidence []struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
		} `json:"evidence"`
	},
	allowed map[string]bool, known map[Evidence]bool, recommendations *int,
) (Sentence, bool) {
	nature := raw.Nature
	if nature == "" {
		nature = natureFact
	}
	if !allowed[nature] {
		return Sentence{}, false
	}
	if nature == natureRecommendation && *recommendations >= maxRecommendations {
		return Sentence{}, false
	}
	sentence := Sentence{Text: raw.Text, Nature: nature}
	for _, cited := range raw.Evidence {
		sentence.Evidence = append(sentence.Evidence,
			Evidence{EntityType: cited.EntityType, EntityID: cited.EntityID})
	}
	// Grounding decides FIRST. Counting an ungrounded recommendation would let
	// one malformed claim spend the quota and suppress the valid advice behind
	// it — the reader loses the advice and is told nothing about why.
	if !claims.Grounded(sentence, known) {
		return Sentence{}, false
	}
	if nature == natureRecommendation {
		*recommendations++
	}
	return sentence, true
}

// orderedSections renders what survived in the spec's order, dropping a section
// left with nothing — the same rule the deterministic floor follows, so a
// reader cannot tell which writer produced their brief from its shape.
func orderedSections(byKind map[crmcontracts.MeetingBriefSectionKind][]Sentence) []Section {
	out := make([]Section, 0, len(specOrder))
	for _, kind := range specSequence {
		if sentences := claims.Dedupe(byKind[kind]); len(sentences) > 0 {
			out = append(out, Section{Kind: kind, Sentences: sentences})
		}
	}
	return out
}

// knownRecords is every record this input carried, as the (type, id) pairs a
// citation must match. A sentence pointing anywhere else is either invented or
// points somewhere the reader cannot go, and neither is worth showing.
func knownRecords(in Input) map[Evidence]bool {
	known := map[Evidence]bool{{EntityType: citeActivity, EntityID: in.ActivityID}: true}
	if in.Deal != nil {
		known[Evidence{EntityType: citeDeal, EntityID: in.Deal.ID}] = true
	}
	for _, attendee := range in.Attendees {
		known[Evidence{EntityType: citePerson, EntityID: attendee.PersonID}] = true
	}
	for _, claim := range in.Commitments {
		known[Evidence{EntityType: citeActivity, EntityID: claim.SourceID}] = true
	}
	for _, act := range in.Recent {
		known[Evidence{EntityType: citeActivity, EntityID: act.ID}] = true
	}
	for _, earlier := range in.PriorMeetings {
		known[Evidence{EntityType: citeActivity, EntityID: earlier.ID}] = true
	}
	// The account history the arc is built from. A conversation this caller may
	// not READ is deliberately absent: it reached the input as a date and a
	// count, and a citation pointing at it would send the reader to a record
	// they cannot open.
	for _, row := range in.History {
		if row.Withheld {
			continue
		}
		known[Evidence{EntityType: citeActivity, EntityID: row.ID}] = true
	}
	return known
}
