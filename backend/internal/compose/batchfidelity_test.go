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
	for name, tc := range map[string]struct {
		req  model.Request
		want []string
	}{
		"capture_classify": {classifyRequest([]unlabeledMessage{{ID: first}, {ID: second}}), []string{"m1", "m2"}},
		"owed_verdict":     {owedRequest([]owedCandidate{{ID: first}, {ID: second}}), []string{"m1", "m2"}},
		"request_settlement": {settleRequest([]settleCandidate{
			{Request: activities.RepliedRequest{RequestID: first}},
			{Request: activities.RepliedRequest{RequestID: second}},
		}), []string{first.String(), second.String()}},
		"capture_counterparty_verdict":    {verdictRequest(capture.PendingCounterparty{ID: first}), []string{first.String()}},
		"capture_confidentiality_verdict": {confidentialityRequest(capture.PendingThread{ID: first}), []string{first.String()}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := answerableIDs(t, tc.req); !slices.Equal(got, tc.want) {
				t.Fatalf("the schema lets the model name %v, want exactly what it sent %v", got, tc.want)
			}
		})
	}
}

// The schema admits the ref this call showed and nothing else, the record id
// behind it included, so a grammar-constrained decoder cannot write either.
func TestTheSchemaRefusesAnythingButTheRefItSent(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	req := classifyRequest([]unlabeledMessage{{ID: id}})
	answer := func(answered string) string {
		return `{"results":[{"id":"` + answered + `","label":"noise","confidence":0.9,"reply":null}]}`
	}
	for _, foreign := range []string{id.String(), "m2"} {
		if err := schema.ValidateJSON(req.ResponseSchema, answer(foreign)); err == nil {
			t.Errorf("the schema admitted %q, which this call never sent", foreign)
		}
	}
	if err := schema.ValidateJSON(req.ResponseSchema, answer("m1")); err != nil {
		t.Errorf("the schema refused the ref this call sent: %v", err)
	}
}
