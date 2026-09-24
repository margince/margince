// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Every adapter answers one question about a reply the output ceiling cut off:
// is it a truncated ANSWER or a failed CALL? The port's answer is the first —
// a model.Response carrying the partial text and model.FinishReasonLength — and
// both of its readers depend on every wire giving it: CompleteStructured's
// "answer briefer" retry and aicert's ungraded run key on that value alone.
//
// A wire that answers the second way instead fails quietly in both places. The
// router classifies the truncation as provider_error and walks the ladder to a
// dearer rung, and a certification run reads it as an outage, re-drives it and
// then aborts the task with no record. A wire that drops the terminal entirely
// is the same defect facing the other way: the half-written body reads as a
// complete one.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// partialAnswer is a document cut mid-value, the shape every truncated reply
// takes whichever wire carried it.
const partialAnswer = `{"answer":"aaaa`

// finishWire is one adapter's spelling of a reply that stopped at the output
// ceiling and of one that finished on its own.
type finishWire struct {
	provider string
	// maxTokens selects the Complete path under test where an adapter has two:
	// anthropic rides SSE above streamedCompleteThreshold.
	maxTokens   int
	contentType string
	truncated   func(text string) string
	finished    func(text string) string
	// withheld holds this wire's spellings of a provider declining to deliver
	// an answer, by name. Empty only for a wire waived in withholdsNothing.
	withheld map[string]string
}

// withholdsNothing names the wires with no withholding terminal at all, and
// why, so the census below can tell an exemption from an omission.
var withholdsNothing = gatekit.Waive(map[string]string{
	providerOllama: "ollama's done_reason is stop, length, load or unload: a local runner has no content filter or refusal terminal between the model and the caller",
})

// wireText is text as a JSON string literal, so a fixture never hand-escapes.
func wireText(t *testing.T, text string) string {
	t.Helper()
	b, err := json.Marshal(text)
	if err != nil {
		t.Fatalf("encoding fixture text: %v", err)
	}
	return string(b)
}

func finishWires(t *testing.T) map[string]finishWire {
	t.Helper()
	q := func(text string) string { return wireText(t, text) }
	anthropicSSE := func(stopReason string) func(string) string {
		return func(text string) string {
			return `data: {"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":3}}}` + "\n\n" +
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":` + q(text) + `}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"` + stopReason + `"},"usage":{"output_tokens":5}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n"
		}
	}
	compatReply := func(finishReason string) func(string) string {
		return func(text string) string {
			return `{"model":"m","choices":[{"finish_reason":"` + finishReason + `","message":{"content":` + q(text) + `}}]}`
		}
	}
	compatWithheld := map[string]string{
		"a content filter": `{"model":"m","choices":[{"finish_reason":"content_filter","native_finish_reason":"SAFETY","message":{"content":""}}]}`,
		"a refusal": `{"model":"m","choices":[{"finish_reason":"content_filter",` +
			`"message":{"content":null,"refusal":"I can't help with that."}}]}`,
		"a refusal under a stop": `{"model":"m","choices":[{"finish_reason":"stop",` +
			`"message":{"content":null,"refusal":"I can't help with that."}}]}`,
	}
	return map[string]finishWire{
		"openai": {
			provider: providerOpenAI, contentType: "application/json",
			truncated: func(text string) string {
				return `{"id":"r","model":"gpt-x","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
					`"output":[{"type":"message","content":[{"type":"output_text","text":` + q(text) + `}]}]}`
			},
			finished: func(text string) string {
				return `{"id":"r","model":"gpt-x","status":"completed",` +
					`"output":[{"type":"message","content":[{"type":"output_text","text":` + q(text) + `}]}]}`
			},
			withheld: map[string]string{
				"a refusal part": `{"id":"r","model":"gpt-x","status":"completed",` +
					`"output":[{"type":"message","content":[{"type":"refusal","refusal":"I can't help with that."}]}]}`,
				"a content filter": `{"id":"r","model":"gpt-x","status":"incomplete","incomplete_details":{"reason":"content_filter"},` +
					`"output":[{"type":"message","content":[{"type":"output_text","text":"par"}]}]}`,
			},
		},
		"openai_compatible": {
			provider: providerOpenAICompatible, contentType: "application/json",
			truncated: compatReply("length"), finished: compatReply("stop"), withheld: compatWithheld,
		},
		"vllm": {
			provider: providerVLLM, contentType: "application/json",
			truncated: compatReply("length"), finished: compatReply("stop"), withheld: compatWithheld,
		},
		"anthropic": {
			provider: providerAnthropic, contentType: "application/json",
			truncated: func(text string) string {
				return `{"model":"claude-test","stop_reason":"max_tokens","content":[{"type":"text","text":` + q(text) + `}]}`
			},
			finished: func(text string) string {
				return `{"model":"claude-test","stop_reason":"end_turn","content":[{"type":"text","text":` + q(text) + `}]}`
			},
			withheld: map[string]string{
				"a refusal": `{"model":"claude-test","stop_reason":"refusal","content":[]}`,
			},
		},
		// Anthropic's docs: a reply that filled the model's context window is
		// to be treated as truncated, exactly like one that hit max_tokens.
		"anthropic context window": {
			provider: providerAnthropic, contentType: "application/json",
			truncated: func(text string) string {
				return `{"model":"claude-test","stop_reason":"model_context_window_exceeded","content":[{"type":"text","text":` + q(text) + `}]}`
			},
			finished: func(text string) string {
				return `{"model":"claude-test","stop_reason":"end_turn","content":[{"type":"text","text":` + q(text) + `}]}`
			},
			withheld: map[string]string{
				"a refusal": `{"model":"claude-test","stop_reason":"refusal","content":[{"type":"text","text":"I"}]}`,
			},
		},
		"anthropic streamed": {
			provider: providerAnthropic, maxTokens: streamedCompleteThreshold + 1, contentType: "text/event-stream",
			truncated: anthropicSSE("max_tokens"),
			finished:  anthropicSSE("end_turn"),
			withheld:  map[string]string{"a refusal": anthropicSSE("refusal")("I")},
		},
		"gemini": {
			provider: providerGemini, contentType: "application/json",
			truncated: func(text string) string {
				return `{"candidates":[{"content":{"parts":[{"text":` + q(text) + `}]},"finishReason":"MAX_TOKENS"}]}`
			},
			finished: func(text string) string {
				return `{"candidates":[{"content":{"parts":[{"text":` + q(text) + `}]},"finishReason":"STOP"}]}`
			},
			withheld: map[string]string{
				"a safety stop":    `{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`,
				"a recitation":     `{"candidates":[{"content":{"parts":[{"text":"par"}]},"finishReason":"RECITATION"}]}`,
				"a blocked prompt": `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`,
			},
		},
		"ollama": {
			provider: providerOllama, contentType: "application/json",
			truncated: func(text string) string {
				return `{"model":"gemma3","message":{"role":"assistant","content":` + q(text) + `},"done":true,"done_reason":"length"}`
			},
			finished: func(text string) string {
				return `{"model":"gemma3","message":{"role":"assistant","content":` + q(text) + `},"done":true,"done_reason":"stop"}`
			},
		},
	}
}

// client is this wire's adapter, pointed at a server that answers with body on
// every request; the second return is every request body it received, so a
// test can read what a retry sent.
func (w finishWire) client(t *testing.T, body string) (model.Client, func() []string) {
	t.Helper()
	handler, received := replyWith(t, w.contentType, body)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Ollama's adapter asks /api/show what the model accepts for `think`
		// before its first chat call. That is not a request the retry sent, so it
		// is answered here and kept out of what the test reads back.
		if r.URL.Path == "/api/show" {
			if _, err := rw.Write([]byte(`{"capabilities":["completion"]}`)); err != nil {
				t.Errorf("writing fixture reply: %v", err)
			}
			return
		}
		handler(rw, r)
	}))
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: w.provider, BaseURL: srv.URL, Model: "m"}, allCloudKeys())
	if err != nil {
		t.Fatalf("building the %s adapter: %v", w.provider, err)
	}
	return client, received
}

// replyWith serves body on every request and keeps each request body, so a
// test can read what a retry sent.
func replyWith(t *testing.T, contentType, body string) (http.HandlerFunc, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var received []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		got := readBody(t, r.Body)
		mu.Lock()
		received = append(received, string(got))
		mu.Unlock()
		w.Header().Set("Content-Type", contentType)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing fixture reply: %v", err)
		}
	}
	return handler, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), received...)
	}
}

func TestEveryAdapterReportsACutOffReplyAsATruncatedAnswer(t *testing.T) {
	for name, wire := range finishWires(t) {
		t.Run(name, func(t *testing.T) {
			client, _ := wire.client(t, wire.truncated(partialAnswer))
			resp, err := client.Complete(context.Background(), model.Request{
				MaxTokens: wire.maxTokens, Messages: []model.Message{{Role: "user", Content: "q"}},
			})
			if err != nil {
				t.Fatalf("a reply cut off at the output ceiling failed the call, so the router reads it as an "+
					"outage and walks the ladder instead of asking for a briefer answer: %v", err)
			}
			if resp.Text != partialAnswer {
				t.Errorf("Text = %q, want the partial text the provider generated and billed: %q", resp.Text, partialAnswer)
			}
			if resp.FinishReason != model.FinishReasonLength {
				t.Errorf("FinishReason = %q, want %q — nothing else tells a caller the body is half-written",
					resp.FinishReason, model.FinishReasonLength)
			}
		})
	}
}

// The other direction: a reply that finished on its own is never reported cut
// off, or every complete answer would be sent back to be shortened.
func TestNoAdapterReportsAFinishedReplyAsTruncated(t *testing.T) {
	for name, wire := range finishWires(t) {
		t.Run(name, func(t *testing.T) {
			client, _ := wire.client(t, wire.finished(`{"answer":"done"}`))
			resp, err := client.Complete(context.Background(), model.Request{
				MaxTokens: wire.maxTokens, Messages: []model.Message{{Role: "user", Content: "q"}},
			})
			if err != nil {
				t.Fatalf("a finished reply failed the call: %v", err)
			}
			if resp.FinishReason == model.FinishReasonLength {
				t.Errorf("a reply that finished on its own was reported as cut off at the output ceiling")
			}
		})
	}
}

// What the value is FOR: the structured retry tells a cut-off model to be
// briefer, and it has to reach that retry over the real wire, not only over a
// fake that scripts the finish reason for it.
func TestEveryAdapterEngagesTheBrieferRetryOnACutOffReply(t *testing.T) {
	for name, wire := range finishWires(t) {
		t.Run(name, func(t *testing.T) {
			client, received := wire.client(t, wire.truncated(partialAnswer))
			r := testRouter(map[Tier]model.Client{TierCheapCloud: client, TierPremium: client},
				&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
			req := structuredReq()
			req.MaxTokens = wire.maxTokens

			_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, req, jsonObjectValidator)
			if err == nil {
				t.Fatal("three truncated answers were accepted as one valid one")
			}
			if !strings.Contains(err.Error(), "cut off") {
				t.Errorf("the terminal error does not name the truncation: %v", err)
			}
			bodies := received()
			if len(bodies) < 2 {
				t.Fatalf("the wire saw %d request(s), so there is no retry to inspect", len(bodies))
			}
			if want := strings.Trim(wireText(t, truncationFeedback), `"`); !strings.Contains(bodies[1], want) {
				t.Errorf("the retry does not tell the model it was cut off.\nretry body: %s", bodies[1])
			}
		})
	}
}

// finishWires is the census the tests above walk, so an adapter missing from it
// goes unchecked; this makes that a failure. The fake is exempt because it
// reports whatever finish reason its script names.
func TestFinishWiresCoverEveryProvider(t *testing.T) {
	covered := map[string]bool{}
	for _, wire := range finishWires(t) {
		covered[wire.provider] = true
	}
	withholds := map[string]bool{}
	for _, wire := range finishWires(t) {
		withholds[wire.provider] = withholds[wire.provider] || len(wire.withheld) > 0
	}
	defer withholdsNothing.AssertAllMatched(t)
	for _, provider := range knownProviders {
		if provider != ProviderFake && !covered[provider] {
			t.Errorf("provider %q has no row in finishWires, so nothing checks how it reports a cut-off reply", provider)
		}
		if provider != ProviderFake && covered[provider] && !withholds[provider] && !withholdsNothing.Waived(t, provider) {
			t.Errorf("provider %q has no withheld fixture and no withholdsNothing waiver, so nothing checks how it reports a refusal", provider)
		}
	}
}

// A refusal, a safety stop or a blocked prompt is the provider's ANSWER about
// this content, not an outage: it must reach the caller as model.ErrOutputWithheld
// and name its terminal for the trace. A wire that returned it as a Response
// hands the caller an empty or partial body as though it were a whole one; one
// that returned a bare error sent the cert lane to re-drive it as a dropped
// connection and then abort the task.
func TestEveryAdapterReportsAWithheldAnswerAsWithheld(t *testing.T) {
	for name, wire := range finishWires(t) {
		for variant, body := range wire.withheld {
			t.Run(name+"/"+variant, func(t *testing.T) {
				client, _ := wire.client(t, body)
				_, err := client.Complete(context.Background(), model.Request{
					MaxTokens: wire.maxTokens, Messages: []model.Message{{Role: "user", Content: "q"}},
				})
				if !errors.Is(err, model.ErrOutputWithheld) {
					t.Fatalf("err = %v, want model.ErrOutputWithheld", err)
				}
				if errors.Is(err, model.ErrRequestRejected) {
					t.Errorf("a withheld answer also reads as a rejected request: %v", err)
				}
				if finishReasonFor("", err) == "" {
					t.Errorf("the error names no terminal, so the stored call cannot say why the answer was withheld: %v", err)
				}
			})
		}
	}
}
