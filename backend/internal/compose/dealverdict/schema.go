// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Action is one change the model proposed.
type Action struct {
	Kind        string            `json:"kind"`
	Date        string            `json:"date"`
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency"`
	QuotedWords string            `json:"quoted_words"`
	Probability int               `json:"probability"`
	StageID     string            `json:"stage_id"`
	Criteria    []CriterionCite   `json:"criteria"`
	Cited       []string          `json:"cited_activity_ids"`
	Reason      string            `json:"reason"`
	Confidence  schema.Confidence `json:"confidence"`
}

// CriterionCite is one exit criterion a stage move says the evidence meets:
// the activity that meets it and the words in it that do.
type CriterionCite struct {
	CriterionID string `json:"criterion_id"`
	ActivityID  string `json:"activity_id"`
	QuotedWords string `json:"quoted_words"`
}

// Payload is the model's whole answer.
type Payload struct {
	Actions []Action `json:"actions"`
}

// Request is the one model call for one deal. A pure function of its input,
// so the certification case sends the request that ships.
//
//promptvoice:exempt the reply is a closed set of field changes and a one-line reason that quotes the customer's evidence; Margince's own sentence on the receipt carries the voice.
func Request(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         SystemFor(fence, promptlang.Rule(lang)),
		Messages:       []model.Message{{Role: "user", Content: Prompt(fence, in)}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: responseSchema(in),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// responseSchema is the shape a constrained decoder keeps to. The cited ids
// are an enum of the activities supplied.
func responseSchema(in Input) json.RawMessage {
	r := refsFor(in)
	cited := r.activities.All()
	// "" is the value of a field that does not apply to the kind.
	stages := append([]string{""}, r.stages.All()...)
	criteria := append([]string{""}, r.criteria.All()...)
	return schema.Must(schema.Object(map[string]schema.Node{
		"actions": schema.Array(schema.Object(map[string]schema.Node{
			"kind":         schema.Enum(KindSetCloseDate, KindSetAmount, KindSetProbability, KindMoveStage),
			"date":         schema.String(),
			"amount":       schema.Integer(),
			"currency":     schema.String(),
			"quoted_words": schema.String(),
			"probability":  schema.Integer(),
			"stage_id":     schema.Enum(stages...),
			"criteria": schema.Array(schema.Object(map[string]schema.Node{
				"criterion_id": schema.Enum(criteria...),
				"activity_id":  schema.Enum(cited...),
				"quoted_words": schema.String(),
			}, "criterion_id", "activity_id", "quoted_words")),
			"cited_activity_ids": schema.Array(schema.Enum(cited...)),
			"reason":             schema.String(),
			"confidence":         schema.Number(),
		}, "kind", "date", "amount", "currency", "quoted_words", "probability", "stage_id", "criteria",
			"cited_activity_ids", "reason", "confidence")),
	}, "actions"))
}

// Validator refuses an answer whose shape is wrong, so the model is asked
// again with the reason. Whether a well-formed action may apply is the
// guards' question, not this one.
func Validator(in Input) ai.Validator {
	return func(text string) error {
		_, err := Parse(text, in)
		return err
	}
}

// Parse decodes and checks the model's answer. The answer names records by
// the refs the request showed; Parse returns each read back to its id.
func Parse(text string, in Input) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
		return Payload{}, fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	if msg := readRefs(&payload, refsFor(in)); msg != "" {
		return Payload{}, errors.New(msg)
	}
	if msg := validatePayload(payload, in); msg != "" {
		return Payload{}, errors.New(msg)
	}
	return payload, nil
}

// validatePayload names the first shape violation, or "" for a clean answer.
func validatePayload(payload Payload, in Input) string {
	known := in.activityByID()
	seen := map[string]bool{}
	for _, action := range payload.Actions {
		if seen[action.Kind] {
			return fmt.Sprintf("kind %q appears twice; propose at most one of each", action.Kind)
		}
		seen[action.Kind] = true
		if msg := validateAction(action, known, in); msg != "" {
			return msg
		}
	}
	return ""
}

func validateAction(action Action, known map[string]Activity, in Input) string {
	if msg := validateActionValue(action, in); msg != "" {
		return msg
	}
	if len(action.Cited) == 0 {
		return "every action cites at least one activity id"
	}
	for _, id := range action.Cited {
		if _, ok := known[id]; !ok {
			return fmt.Sprintf("cited id %q is not one of the supplied activities", id)
		}
	}
	if action.Confidence < 0 || action.Confidence > 1 {
		return fmt.Sprintf("confidence %v is outside 0 to 1", action.Confidence)
	}
	return ""
}

// validateActionValue checks the value an action's kind proposes.
func validateActionValue(action Action, in Input) string {
	switch action.Kind {
	case KindSetCloseDate:
		if _, err := time.Parse(time.DateOnly, action.Date); err != nil {
			return fmt.Sprintf("date %q is not YYYY-MM-DD", action.Date)
		}
	case KindSetAmount:
		if action.Amount <= 0 || len(action.Currency) != 3 || action.QuotedWords == "" {
			return "set_amount needs a positive amount, a three-letter currency and the quoted words"
		}
	case KindSetProbability:
		if action.Probability < 0 || action.Probability > 100 {
			return fmt.Sprintf("probability %d is outside 0 to 100", action.Probability)
		}
	case KindMoveStage:
		return validateMove(action, in)
	default:
		return fmt.Sprintf("kind %q is not set_close_date, set_amount, set_probability or move_stage", action.Kind)
	}
	return ""
}

// validateMove checks a stage move names a later stage and that each
// criterion it claims cites one of the move's own activities.
func validateMove(action Action, in Input) string {
	if !slices.ContainsFunc(in.Later, func(stage StageOption) bool { return stage.ID == action.StageID }) {
		return fmt.Sprintf("stage_id %q is not one of the later stages listed", action.StageID)
	}
	for _, cite := range action.Criteria {
		if !slices.ContainsFunc(in.ExitCriteria, func(c ExitCriterion) bool { return c.ID == cite.CriterionID }) {
			return fmt.Sprintf("criterion_id %q is not one of the exit criteria listed", cite.CriterionID)
		}
		if !slices.Contains(action.Cited, cite.ActivityID) {
			return fmt.Sprintf("criterion %q rests on activity %q, which the move does not cite", cite.CriterionID, cite.ActivityID)
		}
	}
	return ""
}
