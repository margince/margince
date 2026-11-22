// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// noteSchema is a write whose body is optional. Beside it are a required
// nullable field and an optional nullable one, whose nulls must survive.
const noteSchema = `{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["note"]},` +
	`"next_step":{"type":["string","null"]},` +
	`"cleared":{"anyOf":[{"type":"string"},{"type":"null"}]},` +
	`"subject":{"type":"string"},` +
	`"body":{"type":"string"},` +
	`"links":{"type":"array","items":{"type":"object","properties":{"entity_id":{"type":"string"},"label":{"type":"string"}},"required":["entity_id"]}}` +
	`},"required":["kind","next_step"],"additionalProperties":false}`

// Every optional property reaches Gemini required and nullable, at every
// depth, in the caller's order; one that admits null already stays optional.
func TestGeminiIsSentEveryOptionalPropertyAsRequiredAndNullable(t *testing.T) {
	strict, err := GeminiStrictSchema(json.RawMessage(noteSchema))
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
		`},"additionalProperties":false,"required":["kind","next_step","subject","body","links"]}`
	if string(strict) != want {
		t.Errorf("strict schema =\n%s\nwant\n%s", strict, want)
	}
}

// Gemini writes required keys in their listed order. So the caller's required
// keys keep their order ahead of the padded ones. A verdict listed first is then
// decided before the phrase only one verdict may carry.
func TestGeminiKeepsTheCallersRequiredOrderAheadOfThePadding(t *testing.T) {
	strict, err := GeminiStrictSchema(json.RawMessage(`{"type":"object","properties":{` +
		`"confidence":{"type":"number"},"due_at":{"type":"string"},"remaining":{"type":"string"},"verdict":{"type":"string"}},` +
		`"required":["verdict","remaining","confidence"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Required []string }
	if err := json.Unmarshal(strict, &got); err != nil {
		t.Fatal(err)
	}
	if want := []string{"verdict", "remaining", "confidence", "due_at"}; !slices.Equal(got.Required, want) {
		t.Errorf("required = %v, want %v", got.Required, want)
	}
}

// The nulls the strict wire made the decoder write come back out. A null in a
// field the caller declared nullable stays, and so does every value written.
func TestGeminiTakesThePaddedNullsBackOut(t *testing.T) {
	reply := `{"kind":"note","next_step":null,"cleared":null,"subject":"Quiet 27 days","body":null,"links":[{"entity_id":"d1","label":null}]}`
	got := GeminiDropPaddedNulls(json.RawMessage(noteSchema), reply)
	if want := `{"kind":"note","next_step":null,"cleared":null,"subject":"Quiet 27 days","links":[{"entity_id":"d1"}]}`; got != want {
		t.Errorf("reply = %s\nwant    %s", got, want)
	}
	if got := GeminiDropPaddedNulls(json.RawMessage(noteSchema), "not json"); got != "not json" {
		t.Errorf("a reply that is not JSON came back as %q", got)
	}
}

// A step's tool call is read by the branch its tool names. A null in that
// tool's optional argument is dropped, and the other tool's schema is not read.
func TestGeminiReadsAToolCallByTheBranchItNames(t *testing.T) {
	step := `{"anyOf":[` +
		`{"type":"object","properties":{"tool":{"type":"string","enum":["log_activity"]},"args":` + noteSchema + `},"required":["tool","args"],"additionalProperties":false},` +
		`{"type":"object","properties":{"tool":{"type":"string","enum":["read_record"]},"args":{"type":"object","properties":{"body":{"type":["string","null"]}}}},"required":["tool","args"],"additionalProperties":false}` +
		`]}`
	reply := `{"tool":"log_activity","args":{"kind":"note","next_step":null,"subject":"s","body":null,"links":null}}`
	if got, want := GeminiDropPaddedNulls(json.RawMessage(step), reply), `{"tool":"log_activity","args":{"kind":"note","next_step":null,"subject":"s"}}`; got != want {
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

// A schema-constrained request is refused a stream before anything is sent. A
// stream cannot take the padded nulls back out of a reply it hands on.
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

// A property that admits null is one the caller may clear, so its absence and
// its null mean two things, as on a PATCH body. It is not made required: the
// decoder may still leave it out, and a null it writes is the model's.
func TestGeminiLeavesANullablePropertyOptional(t *testing.T) {
	strict, err := GeminiStrictSchema(json.RawMessage(`{"type":"object","properties":{` +
		`"id":{"type":"string"},"note":{"type":["string","null"]},"status":{"type":"string"}},"required":["id"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Required []string }
	if err := json.Unmarshal(strict, &got); err != nil {
		t.Fatal(err)
	}
	if want := []string{"id", "status"}; !slices.Equal(got.Required, want) {
		t.Errorf("required = %v, want %v: a nullable property made required is written as null and clears the field", got.Required, want)
	}
}

// A branch is read by the values it was sent. A padded null is not one of an
// enum's values, yet the branch still matches and the null comes back out.
func TestGeminiMatchesABranchThroughAPaddedEnum(t *testing.T) {
	schema := json.RawMessage(`{"anyOf":[{"type":"object","properties":{"kind":{"type":"string","enum":["a"]},` +
		`"mode":{"type":"string","enum":["x","y"]}},"required":["kind"]}]}`)
	if got, want := GeminiDropPaddedNulls(schema, `{"kind":"a","mode":null}`), `{"kind":"a"}`; got != want {
		t.Errorf("reply = %s, want %s", got, want)
	}
}

// The rewriter pads only where the walk that takes the padding back out reads:
// properties, items and anyOf or oneOf branches. A map's values, a definition
// and an allOf part are sent as the caller wrote them.
func TestGeminiPadsOnlyWhereItCanTakeThePaddingBackOut(t *testing.T) {
	inner := `{"type":"object","properties":{"a":{"type":"string"}}}`
	for name, schema := range map[string]string{
		"a map's values": `{"type":"object","properties":{},"additionalProperties":` + inner + `}`,
		"a definition":   `{"type":"object","properties":{},"$defs":{"part":` + inner + `}}`,
		"an allOf part":  `{"allOf":[` + inner + `]}`,
	} {
		strict, err := GeminiStrictSchema(json.RawMessage(schema))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(strict), `"null"`) {
			t.Errorf("%s was padded where no walk takes it back out: %s", name, strict)
		}
	}
}

// On a PATCH body an absent key keeps a field and a null clears it. Through the
// client, the decoder is not made to write the clearable key. A null the model
// chose to write for it reaches the caller.
func TestAPatchBodyKeepsAbsenceAndANullApartOverTheGeminiWire(t *testing.T) {
	const patch = `{"type":"object","properties":{"id":{"type":"string"},"note":{"type":["string","null"]}},"required":["id"],"additionalProperties":false}`
	for reply, want := range map[string]string{
		`{"id":"1"}`:             `{"id":"1"}`,
		`{"id":"1","note":null}`: `{"id":"1","note":null}`,
	} {
		var sent []byte
		client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			if sent, err = io.ReadAll(r.Body); err != nil {
				t.Error(err)
			}
			body, err := json.Marshal(map[string]any{"candidates": []any{map[string]any{
				"content": map[string]any{"parts": []any{map[string]any{"text": reply}}}, "finishReason": "STOP",
			}}})
			if err != nil {
				t.Error(err)
			}
			if _, err := w.Write(body); err != nil {
				t.Error(err)
			}
		})
		resp, err := client.Complete(context.Background(), model.Request{
			Messages: []model.Message{{Role: "user", Content: "patch"}}, ResponseSchema: json.RawMessage(patch),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Text != want {
			t.Errorf("reply %s reached the caller as %s, want %s", reply, resp.Text, want)
		}
		if strings.Contains(string(sent), `"required":["id","note"]`) {
			t.Errorf("the clearable note was sent required, so the decoder must write it: %s", sent)
		}
	}
}
