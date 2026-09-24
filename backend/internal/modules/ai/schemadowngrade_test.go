// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// schemaDowngradeWire is one adapter under test: how to build it against a
// fake endpoint, and the body that endpoint answers a served call with.
type schemaDowngradeWire struct {
	name   string
	client func(*testing.T, http.HandlerFunc) model.Client
	served string
}

var schemaDowngradeWires = []schemaDowngradeWire{
	{
		"openai", newOpenAIForTest,
		`{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{}"}]}]}`,
	},
	{
		"openai_compatible", newVLLMForTest,
		`{"model":"m","choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`,
	},
}

const (
	// strictSchema meets the strict profile: closed, every property required.
	strictSchema = `{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`
	// openSchema leaves its object open, which the strict profile refuses.
	openSchema = `{"type":"object","properties":{"ok":{"type":"boolean"}}}`
)

// An OpenAI-wire schema the strict profile refuses still goes, with strict
// false, and the Response says so: an answer the endpoint was only shown the
// shape of is not recorded as one it was held to.
func TestAnOpenAIWireReportsASchemaItSentUnenforced(t *testing.T) {
	for _, wire := range schemaDowngradeWires {
		for _, tc := range []struct{ schema, want string }{
			{strictSchema, ""},
			{openSchema, model.SchemaUnenforced},
			{"", ""},
		} {
			client := wire.client(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(wire.served))
			})
			resp, err := client.Complete(context.Background(), model.Request{
				Messages:       []model.Message{{Role: "user", Content: "hi"}},
				ResponseSchema: json.RawMessage(tc.schema),
			})
			if err != nil {
				t.Fatalf("%s: %v", wire.name, err)
			}
			if resp.SchemaDowngrade != tc.want {
				t.Errorf("%s with %q: SchemaDowngrade = %q, want %q", wire.name, tc.schema, resp.SchemaDowngrade, tc.want)
			}
		}
	}
}

// A call that fails after its schema was decided has no Response, so the
// downgrade rides the error — on every adapter that decides one — and the
// sentinel the ladder reads survives the wrapping.
func TestAFailedCallCarriesTheSchemaDowngradeItWasSentUnder(t *testing.T) {
	failing := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"boom"}}`))
	}
	cases := []struct {
		name   string
		client func(*testing.T, http.HandlerFunc) model.Client
		schema string
		want   string
	}{
		{"openai", newOpenAIForTest, openSchema, model.SchemaUnenforced},
		{"openai_compatible", newVLLMForTest, openSchema, model.SchemaUnenforced},
		{"anthropic dropped", newAnthropicForTest, `{"type":"object","properties":{"fields":{"type":"object"}}}`, model.SchemaDropped},
		{
			"anthropic relaxed", newAnthropicForTest,
			`{"type":"object","additionalProperties":false,"properties":{"body":{"type":"string","maxLength":10}},"required":["body"]}`, model.SchemaRelaxed,
		},
		{"anthropic held", newAnthropicForTest, strictSchema, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.client(t, failing).Complete(context.Background(), model.Request{
				Messages:       []model.Message{{Role: "user", Content: "hi"}},
				ResponseSchema: json.RawMessage(tc.schema),
			})
			if err == nil {
				t.Fatal("a 500 was served")
			}
			if got := schemaDowngradeFor("", err); got != tc.want {
				t.Errorf("downgrade on the error = %q, want %q (%v)", got, tc.want, err)
			}
		})
	}
}

// Wrapping an error in its downgrade leaves every sentinel and accessor under
// it readable, and a served call's own report outranks anything on an error.
func TestReportSchemaDowngradeKeepsTheWrappedErrorReadable(t *testing.T) {
	withheld := withheldError{wire: providerOpenAI, reason: finishRefusal}
	_, err := reportSchemaDowngrade(model.Response{}, withheld, model.SchemaUnenforced)
	if !errors.Is(err, model.ErrOutputWithheld) || finishReasonFor("", err) != finishRefusal {
		t.Fatalf("the withheld sentinel or its finish reason was lost in the wrap: %v", err)
	}
	if _, bare := reportSchemaDowngrade(model.Response{}, withheld, ""); schemaDowngradeFor("", bare) != "" {
		t.Fatalf("no downgrade must leave the error without one: %v", bare)
	}
	if got := schemaDowngradeFor(model.SchemaRelaxed, err); got != model.SchemaRelaxed {
		t.Fatalf("the Response's own report lost to the error's: %q", got)
	}
	resp, err := reportSchemaDowngrade(model.Response{Text: "{}"}, nil, model.SchemaDropped)
	if err != nil || resp.SchemaDowngrade != model.SchemaDropped || resp.Text != "{}" {
		t.Fatalf("a served call's downgrade = %+v / %v", resp, err)
	}
}
