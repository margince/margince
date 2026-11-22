// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package searchplan validates model plans before read-only execution.
package searchplan

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Read engines form a closed vocabulary so plans cannot select arbitrary tools.
const (
	EngineReporting   = "reporting"
	EngineAnalytics   = "analytics"
	EngineRecords     = "records"
	EngineUnsupported = "unsupported"
	jsonNull          = "null"
)

// Plan names only readers; a model cannot choose a tool to invoke.
type Plan struct {
	Engine           string   `json:"engine"`
	Arguments        string   `json:"arguments"`
	Count            bool     `json:"count"`
	UnsupportedParts []string `json:"unsupported_parts"`
}

const searchDataSystem = `Translate the question into exactly one typed READ request. Return engine, arguments (a JSON object encoded as a string), and count (boolean). Never answer with a result number or invent a filter or identifier. Also return unsupported_parts: an array naming every phrase you cannot represent in full. Any nonempty unsupported_parts makes the whole question unsupported.
Choose reporting for a standard metric in the supplied catalog. Its arguments are ReportingSelection: {"scope":{"kind":"owner|workspace|team|managed_teams","id":"a supplied scope id when required"},"period":"this_week|last_week|this_month|last_month|this_quarter|last_quarter|last_year|custom","interval":{"start_at":"RFC3339","end_at":"RFC3339"},"target_basis":"month","close_window":"all_open","metrics":["one catalog metric id"],"blocks":[]}. Every key but interval is required: send target_basis, close_window and blocks as shown even when the question does not mention them. Omit interval unless custom. For I/my use scope kind owner and self_user_id from the context. For our or no owner use the supplied default_scope. Use only supplied scopes and short record refs; copy those refs in id fields. Intervals are half-open in the supplied timezone. A year range includes both named years. Use count=true ONLY to count the records of bookings_won (Sales won), not its money value. For other metric values count=false.
Choose analytics for a count, sum or grouping not expressed by a standard metric. Its arguments are the AnalyticsQuery grammar in the supplied vocabulary: entity, measures, group_by, filters, limit, scope_kind, scope_id. entity is one of the vocabulary's entity names exactly as listed, never a bare record type. Each measure is an object: {"fn":"count"} counts rows, {"fn":"sum","field":"<a measure of the entity>"} adds one up. count stays false for analytics; a counting measure is how analytics counts. Never set save. All money filter operands in this planner reply are in MAJOR units; the server alone scales them into the reader's minor units. Include a currency equality filter for native money. The vocabulary states the measure currency rules.
Choose records for a list of matching records. Its arguments are {"plan":<the query vocabulary's typed plan>}, and the plan always names its version and target: {"plan":{"version":"` + search.PlanVersion + `","target":"deal","where":[...]}}. Include every requested condition. For example, deals closing this month over an amount use expected_close_date and the amount field and its declared units. For money thresholds with no named currency use the supplied base_currency. A native amount_minor threshold must also filter currency to that currency. Leave monetary filter operands in MAJOR units, as the vocabulary's money_operands says: 50k is 50000. The server converts to minor units. Never compare unlike currencies. A bounded list is not a total count.
Choose unsupported with arguments="{}" and count=false for handbook questions, ambiguous currencies, unidentified contacts, unsupported filters, or any request that cannot be represented in full. Never silently drop part of a question. Do not guess IDs. Do not write or save anything.`

// Request fences the question and vocabulary to keep them separate from instructions.
//
//promptlang:exempt typed read plans contain no user-facing prose.
//promptvoice:exempt the model returns a typed read plan and no prose for the user.
func Request(question, vocabulary string) model.Request {
	fence := promptfence.New()
	refs := Refs(question, vocabulary)
	return model.Request{
		System:         searchDataSystem + "\n" + fence.Rule("question and vocabulary"),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(promptref.Rewrite(vocabulary+"\nQuestion: "+question, refs))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
		ResponseSchema: schema.Must(schema.Record(
			schema.Field("engine", schema.Enum(EngineReporting, EngineAnalytics, EngineRecords, EngineUnsupported)),
			schema.Field("arguments", schema.String()), schema.Field("count", schema.Boolean()),
			schema.Field("unsupported_parts", schema.Array(schema.String())),
		)),
	}
}

// Decode refuses unknown engines, write projections and malformed argument objects.
func Decode(reply string) (Plan, error) {
	var plan Plan
	if err := DecodeArguments(reply, &plan); err != nil {
		return plan, err
	}
	switch plan.Engine {
	case EngineReporting, EngineAnalytics, EngineRecords, EngineUnsupported:
	default:
		return plan, errors.New("choose a read engine or unsupported")
	}
	if !json.Valid([]byte(plan.Arguments)) || !strings.HasPrefix(strings.TrimSpace(plan.Arguments), "{") {
		return plan, errors.New("arguments must encode one JSON object")
	}
	if plan.Count && plan.Engine != EngineReporting {
		return plan, errors.New("count is only a reporting projection")
	}
	return plan, nil
}

// DecodeArguments accepts one fenced JSON value and refuses undeclared fields.
func DecodeArguments[T any](raw string, out *T) error {
	if hasMultipleDocuments(raw) {
		return errors.New("send exactly one JSON value")
	}
	decoder := json.NewDecoder(strings.NewReader(ai.Unfence(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("send exactly one JSON value")
	}
	return nil
}

func hasMultipleDocuments(document string) bool {
	var first, second json.RawMessage
	stream := json.NewDecoder(strings.NewReader(document))
	return stream.Decode(&first) == nil && stream.Decode(&second) == nil
}
