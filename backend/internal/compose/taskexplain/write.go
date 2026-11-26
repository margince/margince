// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Completer is the model seam: the summarize lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// The natures are the sections the panel draws, spelled from the contract.
const (
	natureFact           = string(crmcontracts.CompanyBriefSentenceNatureFact)
	natureAssessment     = string(crmcontracts.CompanyBriefSentenceNatureAssessment)
	natureRecommendation = string(crmcontracts.CompanyBriefSentenceNatureRecommendation)
)

var knownNature = map[string]bool{
	natureFact: true, natureAssessment: true, natureRecommendation: true,
}

const (
	citeContact  = string(crmcontracts.CompanyBriefEvidenceEntityTypeContact)
	citeActivity = string(crmcontracts.CompanyBriefEvidenceEntityTypeActivity)
	citeDeal     = string(crmcontracts.CompanyBriefEvidenceEntityTypeDeal)
)

// maxSentences keeps the panel a reason, not a second timeline.
const maxSentences = 5

const explainSystem = `You explain to a salesperson why ONE open task is on their list, from a JSON summary of the task, the message it came from and their relationship with the contact.
Return ONLY a JSON object: {"sentences":[{"text":"...","nature":"fact|assessment|recommendation","evidence":[{"entity_type":"contact|deal|activity","entity_id":"<the record's ref, such as a1>"}]}]}.
Write three parts, in this order.
FACTS (one to three): what happened that made this task, and where things stand now. Say who said what, in substance: what was promised, asked or agreed, in their own words where the summary quotes them. Then say whether anything newer in the relationship moved it on, or that nothing has since.
ONE ASSESSMENT: what it costs THIS reader if nothing is done, in terms of this contact, this deal or this promise. Never a sentence that would fit any task.
ONE RECOMMENDATION: the single concrete next step, naming who to contact and what to say or ask. Never "do the task", "complete it", "mark it done" or "move it to another day".
The task's origin says who filed it: "commitment" is a promise read from a message (the task text names who promised), "email_request" is something a customer asked for, and any other origin is a rule or a colleague. A promise made by the CONTACT means the reader is waiting on them; a promise made by the reader means the reader owes it.
Never restate the task's title as a finding: the reader already sees it. Never mention the due date unless the task has one and it matters.
A message may carry a "speaker" with a "kind": "them" is the contact, "you" is the reader, anything else is a third party. Attribute every line to its own speaker and never to anyone else.
A message with "withheld": true may only be said to exist on its date. Never say what it said or who wrote it.
Name a date, an amount or a stage only when the summary supplies it. Never compute one.
Cite the refs the summary gave you, and put them ONLY in evidence, never in the text. A sentence about the task cites the task; one drawn from a message cites that message.
If the summary is too thin to say what happened, write only the facts it does support and stop.
Address the reader as "you". Name the contact once; after that they are "they".`

func systemFor(fence promptfence.Fence, lang string) string {
	return explainSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("task summary")
}

// ExplainRequest builds the one request this site sends. Exported so the
// certification case sends the request production sends.
func ExplainRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         systemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(shownRefs(in).Show(encodeInput(in)))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
		Subjects:       in.subjects(),
		ResponseSchema: explainSchema(),
	}
}

func explainSchema() json.RawMessage {
	return schema.Must(schema.Record(
		schema.Field("sentences", schema.Array(schema.Record(
			schema.Field("text", schema.String()),
			schema.Field("nature", schema.Enum(slices.Sorted(maps.Keys(knownNature))...)),
			schema.Field("evidence", schema.Array(schema.Record(
				schema.Field("entity_type", schema.Enum(citeContact, citeDeal, citeActivity)),
				schema.Field("entity_id", schema.String()),
			))),
		))),
	))
}

func (in Input) subjects() *model.Subjects {
	activities := []string{in.Task.ID}
	if in.Source != nil {
		activities = append(activities, in.Source.ID)
	}
	var contacts []string
	if in.Relationship != nil {
		contacts = append(contacts, in.Relationship.ID)
		for _, act := range in.Relationship.Recent {
			activities = append(activities, act.ID)
		}
		for _, claim := range in.Relationship.Claims {
			activities = append(activities, claim.SourceID)
		}
	}
	return model.SubjectsOf(contacts, activities)
}

// encodeInput cannot fail: Input is plain scalars and slices. An empty prompt
// still reaches the model fenced, and the grounding refuses its reply.
func encodeInput(in Input) string {
	encoded, _ := json.Marshal(in) //nolint:errchkjson // Input is a plain struct of scalars; marshal cannot fail
	return string(encoded)
}

func shownRefs(in Input) claims.Refs {
	return claims.MintRefs(encodeInput(in), knownRecords(in))
}

// Write produces the explanation. The third return says the lane failed, so
// the caller does not cache a floor that an outage wrote.
func Write(ctx context.Context, lane Completer, in Input, lang string) ([]claims.Sentence, crmcontracts.WrittenBy, bool) {
	floor := Floor(in, lang)
	if lane == nil {
		return floor, crmcontracts.WrittenByDeterministic, false
	}
	written, err := writeWithModel(ctx, lane, in, lang)
	if err != nil {
		// The declared degrade: the reader gets the facts, and generated_by says so.
		return floor, crmcontracts.WrittenByDeterministic, true
	}
	return written, crmcontracts.WrittenByModel, false
}

func writeWithModel(ctx context.Context, lane Completer, in Input, lang string) ([]claims.Sentence, error) {
	resp, err := ai.Ask(ctx, lane, ExplainRequest(in, lang), func(text string) error {
		_, err := ParseExplanation(text, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	kept, err := ParseExplanation(resp.Text, in)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		return nil, errors.New("the task explanation reply kept no grounded sentence")
	}
	return kept, nil
}

// ParseExplanation reads a reply into grounded sentences, dropping any that
// cite a record the input did not carry or that says what any task could.
func ParseExplanation(reply string, in Input) ([]claims.Sentence, error) {
	var parsed struct {
		Sentences []claims.Sentence `json:"sentences"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return nil, fmt.Errorf("parse the task explanation reply: %w", err)
	}
	read := shownRefs(in).Read(parsed.Sentences)
	specific := slices.DeleteFunc(read, func(sentence claims.Sentence) bool { return Generic(sentence.Text) })
	return claims.KeepBrief(specific, knownRecords(in), claims.BriefBounds{
		Sentences:       maxSentences,
		Recommendations: 1,
		Natures:         knownNature,
		Fact:            natureFact,
		Recommendation:  natureRecommendation,
	}), nil
}
