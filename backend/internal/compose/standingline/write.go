// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
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

const (
	natureFact   = string(crmcontracts.CompanyBriefSentenceNatureFact)
	citeActivity = string(crmcontracts.CompanyBriefEvidenceEntityTypeActivity)
)

const lineSystem = `You write the one sentence a CRM shows under "How it stands" for one record, from a JSON list of its newest activities, newest first.
Return ONLY a JSON object: {"keep_previous":true|false,"sentences":[{"text":"...","evidence":[{"entity_type":"activity","entity_id":"<the activity's ref, such as a1>"}]}]}.
The sentence says where things stand now, in substance, as a colleague would: who said or did what last and what that means for the next step. For example: "In her last mail on 24 November, Anna Weber confirmed the rollout plan and asked for the contract draft by Friday."
Read the newest activity first. Use an older one only when the newest makes no sense without it.
Name people by the name the summary gives. The line is read by several colleagues, so never write "you" or "we". "side": "customer" is the customer's side, "our team" is a colleague.
Never say only who wrote or how often ("both sides have written", "there was an exchange", "they replied"). Say what was said.
Name a date only as the summary gives it. Never compute one. Never invent a promise, an amount or a stage.
If "previous_line" is present and still says correctly where things stand, return {"keep_previous":true,"sentences":[]}.
If the activities say too little to tell where things stand, return {"keep_previous":false,"sentences":[]}.
Cite the activities the sentence rests on by their refs, ONLY in evidence, never in the text. At most 30 words.`

func systemFor(fence promptfence.Fence, lang string) string {
	return lineSystem + "\n" + promptlang.Rule(lang) + "\n" + fence.Rule("activity summary")
}

// LineRequest builds the one request this site sends. Exported so the
// certification case sends the request production sends.
func LineRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         systemFor(fence, lang),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(shownRefs(in).Show(encodeInput(in)))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
		Subjects:       model.SubjectsOf(nil, in.Activities()),
		ResponseSchema: lineSchema(),
	}
}

func lineSchema() json.RawMessage {
	return schema.Must(schema.Record(
		schema.Field("keep_previous", schema.Boolean()),
		schema.Field("sentences", schema.Array(schema.Record(
			schema.Field("text", schema.String()),
			schema.Field("evidence", schema.Array(schema.Record(
				schema.Field("entity_type", schema.Enum(citeActivity)),
				schema.Field("entity_id", schema.String()),
			))),
		))),
	))
}

// encodeInput cannot fail: Input is plain scalars and slices.
func encodeInput(in Input) string {
	encoded, _ := json.Marshal(in) //nolint:errchkjson // Input is a plain struct of scalars; marshal cannot fail
	return string(encoded)
}

func shownRefs(in Input) claims.Refs {
	return claims.MintRefs(encodeInput(in), knownRecords(in))
}

// Answer is what one check of a record decided.
type Answer struct {
	// Keep says the shown line still holds.
	Keep bool
	// Text is the new line. Empty with Keep false: the activities say too
	// little, and the page keeps its own reading.
	Text string
}

// Write asks the lane whether the line still holds, and for a new one if not.
func Write(ctx context.Context, lane Completer, in Input, lang string) (Answer, error) {
	if lane == nil || len(in.Recent) == 0 {
		return Answer{}, errors.New("a status line needs a model lane and an activity to read")
	}
	resp, err := ai.Ask(ctx, lane, LineRequest(in, lang), func(text string) error {
		_, err := ParseLine(text, in)
		return err
	})
	if err != nil {
		return Answer{}, err
	}
	return ParseLine(resp.Text, in)
}

// ParseLine reads a reply, keeping its sentence only when it cites what the
// record's activities carry and says more than stock copy.
func ParseLine(reply string, in Input) (Answer, error) {
	var parsed struct {
		KeepPrevious bool              `json:"keep_previous"`
		Sentences    []claims.Sentence `json:"sentences"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return Answer{}, fmt.Errorf("parse the status line reply: %w", err)
	}
	if parsed.KeepPrevious && in.Previous != "" {
		return Answer{Keep: true}, nil
	}
	read := shownRefs(in).Read(parsed.Sentences)
	specific := slices.DeleteFunc(read, func(sentence claims.Sentence) bool { return Generic(sentence.Text) })
	kept := claims.KeepBrief(specific, knownRecords(in), claims.BriefBounds{
		Sentences: 1, Natures: map[string]bool{natureFact: true}, Fact: natureFact,
	})
	if len(kept) == 0 {
		return Answer{}, nil
	}
	return Answer{Text: kept[0].Text}, nil
}

// stockPhrases are status lines that would fit any record: who wrote, never
// what was said.
var stockPhrases = []string{
	// English
	"both sides have written", "both sides wrote", "there was an exchange", "there has been an exchange",
	"only they have written", "only we have written", "no exchange yet", "the relationship is active",
	// German
	"beide seiten haben geschrieben", "es gab einen austausch", "noch kein austausch",
	// Vietnamese
	"cả hai bên đã viết", "đã có trao đổi", "chưa có trao đổi",
}

// Generic says whether a line is stock copy rather than about this record.
func Generic(text string) bool {
	return claims.SaysStock(text, stockPhrases)
}
