// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package filterpropose reads a list described in plain words into filter
// clauses a human then edits and saves.
//
// THE MODEL PROPOSES, THE ENGINE DECIDES. The model is handed the record type,
// the sentence, the caller's own filter vocabulary, today's date and the
// reader's language, and never a record. What it answers is a predicate tree,
// and membership is decided later by the predicate engine evaluating that tree
// exactly as it evaluates one a human built. Nothing here saves anything.
//
// Every clause passes Gate before a reader sees it. A clause the vocabulary
// cannot express is dropped and named back as unsupported, so one bad phrase
// never costs the reader the rest of the proposal.
package filterpropose

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Field is one entry of the caller's filter vocabulary, as the model reads it
// and as Gate checks against it. It is the vocabulary read's own shape plus the
// admin's label for a custom field, which is what lets "sales region" find
// cf_sales_region.
type Field struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Operators []string `json:"operators"`
	Options   []string `json:"options,omitempty"`
	Custom    bool     `json:"custom,omitempty"`
	Label     string   `json:"label,omitempty"`
	Currency  string   `json:"currency,omitempty"`
}

// Input is everything one proposal is read from.
type Input struct {
	Resource string
	Text     string
	Fields   []Field
	Today    time.Time
	// Lang is the language the unsupported reasons are written in.
	Lang string
}

// Clause is one condition the model proposed, with the words it was read from.
// Exactly one value slot carries the operand; which one depends on the field's
// type and the operator, and Gate is where that is decided.
type Clause struct {
	Phrase  string   `json:"phrase"`
	Field   string   `json:"field"`
	Op      string   `json:"op"`
	Text    *string  `json:"text"`
	Number  *float64 `json:"number"`
	Flag    *bool    `json:"flag"`
	List    []string `json:"list"`
	DaysAgo *int     `json:"days_ago"`
}

// Group is clauses joined one way.
type Group struct {
	Join    string   `json:"join"`
	Clauses []Clause `json:"clauses"`
}

// Unsupported is a phrase the model found no way to express.
type Unsupported struct {
	Phrase string `json:"phrase"`
	Reason string `json:"reason"`
}

// Answer is the model's reply: groups joined one way at the root. Two levels
// are enough for every sentence a human types ("in Germany or Austria, with no
// activity since March"), and a flat shape is one every provider's constrained
// decoder can hold, which a recursive schema is not.
type Answer struct {
	Groups      []Group       `json:"groups"`
	Join        string        `json:"join"`
	Unsupported []Unsupported `json:"unsupported"`
}

const joinAnd, joinOr = "and", "or"

var operators = []string{
	storekit.OpEq, storekit.OpNeq, storekit.OpGt, storekit.OpGte, storekit.OpLt,
	storekit.OpLte, storekit.OpIn, storekit.OpContains, storekit.OpExists,
}

const systemPrompt = `You turn a CRM user's description of a list into filter conditions over one record type.

You are given the fields this user may filter on: each has a name, a type, the operators it accepts and, for a picklist, its options. Use ONLY those fields, operators and options. You never see records; the conditions you write are evaluated later by the CRM itself.

Each condition names a field, an operator and ONE value slot:
- text: text, picklist, multiselect, domain and id values, and a fixed date as YYYY-MM-DD.
- number: number values, and currency amounts in MAJOR units (50000 for fifty thousand euros).
- flag: true or false, for boolean fields and for the "exists" operator (exists true = has a value, exists false = empty).
- list: the values for the "in" operator.
- days_ago: a whole number of days counted back from today, for relative dates, and only with gt, gte, lt or lte. "In the last 45 days" is gte days_ago 45; "more than 45 days ago" is lt days_ago 45.
Set every other slot to null. Put the words each condition was read from in phrase.

"No activity in the last N days", in any language, means last_activity_at lt days_ago N OR last_activity_at exists false: a record nobody ever contacted has no activity either.
A country is ALWAYS a two-letter ISO 3166 code, never its name: Germany is DE, Austria is AT, Switzerland is CH.
A picklist value must be one of its options, spelled exactly as listed.

Group conditions: each group joins its clauses with "and" or "or", and join says how the groups combine. Alternatives for one field ("Germany or Austria") belong in one "or" group or one "in" condition. An "or" inside a condition that also has other requirements is its own group. "Companies in Berlin with no activity in 10 days" is TWO groups under join "and": {"groups":[{"join":"and","clauses":[{"phrase":"in Berlin","field":"city","op":"eq","text":"Berlin"}]},{"join":"or","clauses":[{"phrase":"no activity in 10 days","field":"last_activity_at","op":"lt","days_ago":10},{"phrase":"never contacted","field":"last_activity_at","op":"exists","flag":false}]}],"join":"and"} (slots not shown are null). Never put an alternative into an "and" group, and never leave a text, list, number or flag slot null when the operator needs it.

A phrase no field can express - an opinion, a prediction, a fact the fields do not record, a specific contact, company or colleague you cannot name by id - goes in unsupported, with a one-sentence reason. Never guess a field for it, and never drop it silently.`

// Request builds the model call.
//
// The vocabulary is fenced as well as the sentence: a custom field's label and
// options are text an admin typed, and interpolated bare they would be read in
// the prompt's own voice.
//
//promptvoice:exempt the reply is a filter tree; its only prose is a one-sentence reason per unused phrase, which names a field rather than speaking for the product.
func Request(in Input) model.Request {
	fence := promptfence.New()
	vocabulary, err := json.Marshal(in.Fields)
	if err != nil {
		// A slice of plain structs always encodes; an error here is a broken
		// build, and an empty vocabulary is the answer that proposes nothing.
		vocabulary = []byte("[]")
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Record type: %s\nToday: %s\n\n", in.Resource, in.Today.Format(time.DateOnly))
	prompt.WriteString("Fields (untrusted):\n" + fence.Wrap(string(vocabulary)) + "\n\n")
	prompt.WriteString("The list the user described (untrusted):\n" + fence.Wrap(in.Text) + "\n\n")
	prompt.WriteString(`Return JSON: { "groups": [ { "join", "clauses": [ { "phrase", "field", "op", "text", "number", "flag", "list", "days_ago" } ] } ], "join", "unsupported": [ { "phrase", "reason" } ] }`)
	return model.Request{
		System: systemPrompt + "\n\n" + promptlang.Rule(in.Lang) + "\n" +
			fence.Rule("user-supplied"),
		Messages:       []model.Message{{Role: "user", Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: Schema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// Schema is the shape the model must answer in. Field names are a plain string
// rather than a per-call enum: an unknown name is Gate's to refuse and name
// back, which an enum would turn into a guess at the nearest real field.
func Schema() json.RawMessage {
	clause := schema.Record(
		schema.Field("phrase", schema.String()),
		schema.Field("field", schema.String()),
		schema.Field("op", schema.Enum(operators...)),
		schema.Field("text", schema.Optional(schema.String())),
		schema.Field("number", schema.Optional(schema.Number())),
		schema.Field("flag", schema.Optional(schema.Boolean())),
		schema.Field("list", schema.Optional(schema.Array(schema.String()))),
		schema.Field("days_ago", schema.Optional(schema.Integer())),
	)
	group := schema.Record(
		schema.Field("join", schema.Enum(joinAnd, joinOr)),
		schema.Field("clauses", schema.Array(clause)),
	)
	return schema.Must(schema.Record(
		schema.Field("groups", schema.Array(group)),
		schema.Field("join", schema.Enum(joinAnd, joinOr)),
		schema.Field("unsupported", schema.Array(schema.Record(
			schema.Field("phrase", schema.String()),
			schema.Field("reason", schema.String()),
		))),
	))
}

// Parse decodes the model's answer. It reads the shape and nothing more; which
// clauses deserve to reach a reader is Gate's question.
func Parse(raw string) (Answer, error) {
	var answer struct {
		Groups      *[]Group      `json:"groups"`
		Join        string        `json:"join"`
		Unsupported []Unsupported `json:"unsupported"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(raw)), &answer); err != nil {
		return Answer{}, fmt.Errorf("filterpropose: the reply is not the shape this site takes: %w", err)
	}
	// Required by the schema, so absent means malformed rather than empty: read
	// as empty, a provider answering `{}` would report a sentence as unreadable.
	if answer.Groups == nil {
		return Answer{}, fmt.Errorf("filterpropose: the reply carries no groups field, which the schema requires")
	}
	if answer.Join != joinAnd && answer.Join != joinOr {
		return Answer{}, fmt.Errorf("filterpropose: the reply joins its groups with %q, not and/or", answer.Join)
	}
	return Answer{Groups: *answer.Groups, Join: answer.Join, Unsupported: answer.Unsupported}, nil
}
