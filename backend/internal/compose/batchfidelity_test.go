// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// answerableIDs reads the ids a request's schema lets the model name: the enum
// on results[].id.
func answerableIDs(t *testing.T, req model.Request) []string {
	t.Helper()
	var shape struct {
		Properties map[string]struct {
			Items struct {
				Properties struct {
					ID struct {
						Enum []string `json:"enum"`
					} `json:"id"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(req.ResponseSchema, &shape); err != nil {
		t.Fatalf("decoding the response schema: %v", err)
	}
	if len(shape.Properties) != 1 {
		t.Fatalf("want one results array in the schema, got %d properties", len(shape.Properties))
	}
	for _, results := range shape.Properties {
		return results.Items.Properties.ID.Enum
	}
	return nil
}

// Every site that asks the model to answer per id gives the decoder exactly the
// ids it sent, built by the site's own request builder — the request that
// ships, not a schema assembled beside it.
func TestEverySiteLetsTheModelNameOnlyTheIDsItWasSent(t *testing.T) {
	t.Parallel()
	first, second := ids.NewV7(), ids.NewV7()
	both := []string{first.String(), second.String()}
	for name, tc := range map[string]struct {
		req  model.Request
		want []string
	}{
		"capture_classify": {classifyRequest([]unlabeledMessage{{ID: first}, {ID: second}}), both},
		"owed_verdict":     {owedRequest([]owedCandidate{{ID: first}, {ID: second}}), both},
		"request_settlement": {settleRequest([]settleCandidate{
			{Request: activities.RepliedRequest{RequestID: first}},
			{Request: activities.RepliedRequest{RequestID: second}},
		}), both},
		"capture_counterparty_verdict":    {verdictRequest(capture.PendingCounterparty{ID: first}), both[:1]},
		"capture_confidentiality_verdict": {confidentialityRequest(capture.PendingThread{ID: first}), both[:1]},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := answerableIDs(t, tc.req); !slices.Equal(got, tc.want) {
				t.Fatalf("the schema lets the model name %v, want exactly the ids sent %v", got, tc.want)
			}
		})
	}
}

// The failure the enum exists for: a small model copies the first half of the
// id it was given and invents the rest. The schema refuses it, so a
// grammar-constrained decoder cannot write it; the real id still passes.
func TestTheSchemaRefusesAHalfCopiedID(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	sent := id.String()
	halfCopied := sent[:19] + "a171-01bcf53a7b35"
	req := classifyRequest([]unlabeledMessage{{ID: id}})
	answer := func(answered string) string {
		return `{"results":[{"id":"` + answered + `","label":"noise","confidence":0.9,"reply":null}]}`
	}
	if err := schema.ValidateJSON(req.ResponseSchema, answer(halfCopied)); err == nil {
		t.Errorf("the schema admitted %q, an id this call never sent", halfCopied)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, answer(sent)); err != nil {
		t.Errorf("the schema refused the id this call sent: %v", err)
	}
}
