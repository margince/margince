// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// abandonedDraft is the partial answer a broker returns when its upstream broke
// off at the first line break, as a structured draft does.
const abandonedDraft = `{"subject":"Ihr Angebot","body":"Hallo Priya,"`

// A model that began its answer and then failed is a finding about the model,
// and the error says how far it got without repeating what it wrote.
func TestOpenAICompatNamesAnAnswerTheUpstreamAbandoned(t *testing.T) {
	body := `{"model":"m","choices":[{"finish_reason":"error","native_finish_reason":"error","message":{"content":` +
		wireText(t, abandonedDraft) + `}}]}`
	c := &openAICompatClient{http: &http.Client{}, defaultModel: "m", baseURL: newJSONServer(t, body)}
	_, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, ErrAnswerAbandoned) {
		t.Fatalf("err = %v, want ErrAnswerAbandoned", err)
	}
	if want := fmt.Sprintf("after %d characters of output", utf8.RuneCountInString(abandonedDraft)); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to state the output length (%q)", err, want)
	}
	if strings.Contains(err.Error(), "Priya") {
		t.Errorf("err = %q carries the abandoned answer, which may be customer data", err)
	}
}

// A failure before any output is indistinguishable from an outage, so it is not
// named as an abandoned answer.
func TestOpenAICompatAFailureBeforeAnyOutputIsNotAnAbandonedAnswer(t *testing.T) {
	c := &openAICompatClient{http: &http.Client{}, defaultModel: "m", baseURL: newJSONServer(t,
		`{"model":"m","choices":[{"finish_reason":"error","message":{"content":""}}]}`)}
	_, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("a failed generation with no output ended cleanly")
	}
	if errors.Is(err, ErrAnswerAbandoned) {
		t.Errorf("err = %v names an abandoned answer, but nothing was produced", err)
	}
}

// A stream counts what its earlier chunks delivered, so the same failure reads
// the same whether the answer arrived whole or in pieces.
func TestOpenAICompatStreamNamesAnAnswerItAbandoned(t *testing.T) {
	wire := streamWires(t)["openai_compatible"]
	_, err := drain(t, wire.stream(t, wire.body(streamedChunks, endDropped)+wire.failed))
	if !errors.Is(err, ErrAnswerAbandoned) {
		t.Fatalf("err = %v, want ErrAnswerAbandoned", err)
	}
	if want := fmt.Sprintf("after %d characters of output", utf8.RuneCountInString(strings.Join(streamedChunks, ""))); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to count every chunk delivered (%q)", err, want)
	}

	_, err = drain(t, wire.stream(t, wire.failed))
	if err == nil || errors.Is(err, ErrAnswerAbandoned) {
		t.Errorf("err = %v, want a plain failure for a stream that failed before any output", err)
	}
}
