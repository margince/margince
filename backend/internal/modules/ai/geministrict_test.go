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

// noteSchema is a write whose body is optional, beside a field that is required
// and nullable and one that is optional and nullable, whose nulls must survive.
const noteSchema = `{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["note"]},` +
	`"next_step":{"type":["string","null"]},` +
	`"cleared":{"anyOf":[{"type":"string"},{"type":"null"}]},` +
	`"subject":{"type":"string"},` +
	`"body":{"type":"string"},` +
	`"links":{"type":"array","items":{"type":"object","properties":{"entity_id":{"type":"string"},"label":{"type":"string"}},"required":["entity_id"]}}` +
	`},"required":["kind","next_step"],"additionalProperties":false}`

// Every optional property reaches Gemini required and nullable, at every
// depth, in the caller's order; one that admits null already is left alone.
func TestGeminiIsSentEveryOptionalPropertyAsRequiredAndNullable(t *testing.T) {
	strict, err := geminiStrictSchema(json.RawMessage(noteSchema))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"object","properties":{` +
		`"kind":{"type":"string","enum":["note"]},` +
		`"next_step":{"type":["string","null"]},` +
		`"cleared":{"anyOf":[{"type":"string"},{"type":"null"}]},` +
		`"subject":{"anyOf":[{"type":"string"},{"type":"null"}]},` +
		`"body":{"anyOf":[{"type":"string"},{"type":"null"}]},` +
		`"links":{"anyOf":[{"type":"array","items":{"type":"object","properties":{"entity_id":{"type":"string"},` +
		`"label":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["entity_id","label"]}},{"type":"null"}]}` +
		`},"additionalProperties":false,"required":["kind","next_step","cleared","subject","body","links"]}`
	if string(strict) != want {
		t.Errorf("strict schema =\n%s\nwant\n%s", strict, want)
	}
}

// The nulls the strict wire made the decoder write come back out; a null in a
// field the caller declared nullable, and every value written, stays.
func TestGeminiTakesThePaddedNullsBackOut(t *testing.T) {
	reply := `{"kind":"note","next_step":null,"cleared":null,"subject":"Quiet 27 days","body":null,"links":[{"entity_id":"d1","label":null}]}`
	got := geminiDropPaddedNulls(json.RawMessage(noteSchema), reply)
	if want := `{"kind":"note","next_step":null,"cleared":null,"subject":"Quiet 27 days","links":[{"entity_id":"d1"}]}`; got != want {
		t.Errorf("reply = %s\nwant    %s", got, want)
	}
	if got := geminiDropPaddedNulls(json.RawMessage(noteSchema), "not json"); got != "not json" {
		t.Errorf("a reply that is not JSON came back as %q", got)
	}
}

// A step's tool call is read by the branch its tool names, so a null in one
// tool's optional argument is dropped and the other tool's schema is not read.
func TestGeminiReadsAToolCallByTheBranchItNames(t *testing.T) {
	step := `{"anyOf":[` +
		`{"type":"object","properties":{"tool":{"type":"string","enum":["log_activity"]},"args":` + noteSchema + `},"required":["tool","args"],"additionalProperties":false},` +
		`{"type":"object","properties":{"tool":{"type":"string","enum":["read_record"]},"args":{"type":"object","properties":{"body":{"type":["string","null"]}}}},"required":["tool","args"],"additionalProperties":false}` +
		`]}`
	reply := `{"tool":"log_activity","args":{"kind":"note","next_step":null,"subject":"s","body":null,"links":null}}`
	if got, want := geminiDropPaddedNulls(json.RawMessage(step), reply), `{"tool":"log_activity","args":{"kind":"note","next_step":null,"subject":"s"}}`; got != want {
		t.Errorf("reply = %s\nwant    %s", got, want)
	}
}

// The client sends the strict schema and answers its caller without the padding.
func TestTheGeminiClientAnswersWithoutThePadding(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"kind\":\"note\",\"next_step\":null,\"subject\":\"s\",\"body\":null,\"links\":null}"}]},"finishReason":"STOP"}]}`))
	})
	resp, err := client.Complete(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "hi"}}, ResponseSchema: json.RawMessage(noteSchema),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"kind":"note","next_step":null,"subject":"s"}`; resp.Text != want {
		t.Errorf("text = %s, want %s", resp.Text, want)
	}
}

// A schema-constrained request is refused a stream before anything is sent: a
// stream cannot have the padded nulls taken back out of a reply it hands on.
func TestGeminiRefusesToStreamASchemaConstrainedRequest(t *testing.T) {
	sent := false
	client := newGeminiForTest(t, func(http.ResponseWriter, *http.Request) { sent = true })
	_, err := client.Stream(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "hi"}}, ResponseSchema: json.RawMessage(noteSchema),
	})
	if !errors.Is(err, errGeminiSchemaStream) || sent {
		t.Errorf("err = %v, request sent %v; want errGeminiSchemaStream and nothing sent", err, sent)
	}
}
