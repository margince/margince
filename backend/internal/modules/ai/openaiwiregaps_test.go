// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where the OpenAI-wire adapters departed from OpenAI's API reference. Covers
// the chat wire's output ceiling and reasoning effort. Also the Responses
// wire's input shapes and a streamed refusal.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func chatBodyFor(t *testing.T, baseURL string, req model.Request) map[string]json.RawMessage {
	t.Helper()
	c := &openAICompatClient{baseURL: baseURL, defaultModel: "m"}
	raw, err := json.Marshal(c.chatWire(req, false))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTheChatWireNamesTheOutputCeilingByWhatItsTargetReads(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ baseURL, model, sent string }{
		"OpenAI's own host, any model":       {"https://api.openai.com", "gpt-4o", "max_completion_tokens"},
		"Azure OpenAI by its host":           {"https://res.openai.azure.com", "o3-mini", "max_completion_tokens"},
		"vLLM serving a reasoning model id":  {"http://vllm:8000", "o3", "max_tokens"},
		"a gateway serving a gpt-5 id":       {"https://gw.example", "gpt-5-mini", "max_tokens"},
		"a lookalike of Azure's host":        {"https://res.openai.azure.com.evil.test", "o3", "max_tokens"},
		"vLLM serving an open model":         {"http://vllm:8000", "qwen3-32b", "max_tokens"},
		"a broker serving a vendor-prefixed": {"https://openrouter.ai/api", "openai/o3", "max_tokens"},
		"a lookalike of OpenAI's host":       {"https://api.openai.com.evil.test", "llama", "max_tokens"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := chatBodyFor(t, tc.baseURL, model.Request{Model: tc.model, MaxTokens: 700})
			if got := string(body[tc.sent]); got != "700" {
				t.Errorf("%s = %q, want 700 (body %v)", tc.sent, got, body)
			}
			other := "max_tokens"
			if tc.sent == other {
				other = "max_completion_tokens"
			}
			if _, both := body[other]; both {
				t.Errorf("%s sent beside %s", other, tc.sent)
			}
		})
	}
}

func TestTheChatWireSendsReasoningEffortOnlyToOpenAIFromTheSameTable(t *testing.T) {
	t.Parallel()
	const openAI = "https://api.openai.com"
	for name, tc := range map[string]struct {
		baseURL, model, level, floor, want string
	}{
		"an admin level":                      {openAI, "gpt-5-mini", "high", "", `"high"`},
		"a floor above the default":           {openAI, "gpt-5.5", "", "high", `"high"`},
		"a floor the default already meets":   {openAI, "gpt-5.5", "", "low", ""},
		"a level the family refuses, raised":  {openAI, "gpt-6.1-sol", "minimal", "", `"low"`},
		"a model that does not reason":        {openAI, "gpt-4o", "high", "", ""},
		"a gateway, which knows no such knob": {"https://gw.example", "gpt-5-mini", "high", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := chatBodyFor(t, tc.baseURL, model.Request{Model: tc.model, ThinkingLevel: tc.level, ThinkingFloor: tc.floor})
			if got := string(body["reasoning_effort"]); got != tc.want {
				t.Errorf("reasoning_effort = %q, want %q", got, tc.want)
			}
			if native := responsesEffort(t, tc.model, tc.level, tc.floor); tc.baseURL == openAI && native != strings.Trim(tc.want, `"`) {
				t.Errorf("the Responses wire sent effort %q for the same request", native)
			}
		})
	}
}

func responsesEffort(t *testing.T, modelID, level, floor string) string {
	t.Helper()
	var r struct {
		Effort string `json:"effort"`
	}
	raw := openaiRequestBody(t, model.Request{Model: modelID, ThinkingLevel: level, ThinkingFloor: floor})["reasoning"]
	if raw != nil {
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
	}
	return r.Effort
}

func TestAnAssistantTurnGoesToTheResponsesAPIAsAString(t *testing.T) {
	var input []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	raw := marshalInput(t, []model.Message{
		{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}, {Role: "user", Content: "q2"},
	})
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if got := string(input[1].Content); got != `"a"` {
		t.Errorf("assistant content = %s, want the bare string", got)
	}
	if got := string(input[0].Content); !strings.Contains(got, `"type":"input_text"`) {
		t.Errorf("user content = %s, want input_text parts", got)
	}
	if strings.Contains(string(raw), "output_text") {
		t.Errorf("an output_text part, which only a full output message takes, was sent: %s", raw)
	}
}

func marshalInput(t *testing.T, msgs []model.Message) []byte {
	t.Helper()
	raw, err := json.Marshal(openaiInputMessages("", msgs, nil))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestResponsesImagesCarryDetailAndInlineFilesCarryAName(t *testing.T) {
	var body []byte
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "look"}},
		Attachments: []model.Attachment{
			{MIME: "image/png", URI: "https://cdn.example/img.png"},
			{MIME: "application/pdf", Bytes: pdfSample},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Input []struct {
			Content []map[string]string `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	parts := wire.Input[0].Content
	if parts[1]["type"] != "input_image" || parts[1]["detail"] != "auto" {
		t.Errorf("image part = %v, want detail auto", parts[1])
	}
	if parts[2]["type"] != "input_file" || parts[2]["filename"] == "" {
		t.Errorf("inline file part = %v, want a filename", parts[2])
	}
}

func streamOf(t *testing.T, events ...string) (string, error) {
	t.Helper()
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		for _, ev := range events {
			_, _ = io.WriteString(w, "data: "+ev+"\n\n")
		}
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	})
	var got strings.Builder
	for {
		chunk, ok, err := stream.Next(context.Background())
		got.WriteString(chunk)
		if err != nil || !ok {
			return got.String(), err
		}
	}
}

func TestAStreamedRefusalEndsAsAWithheldAnswer(t *testing.T) {
	for name, events := range map[string][]string{
		"done event": {
			`{"type":"response.refusal.delta","delta":"I can"}`,
			`{"type":"response.refusal.done","refusal":"I cannot help"}`,
			`{"type":"response.completed"}`,
		},
		"deltas, then completed": {
			`{"type":"response.refusal.delta","delta":"I cannot"}`,
			`{"type":"response.completed"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			text, err := streamOf(t, events...)
			if !errors.Is(err, model.ErrOutputWithheld) {
				t.Errorf("a streamed refusal ended as %q, %v; want ErrOutputWithheld", text, err)
			}
		})
	}
}

// A bare 429 with a reset header is a throttle on OpenAI's adapter. On every
// other provider it stays unclassified, as before.
func TestOnlyOpenAIsAdapterReadsTheRateLimitResetHeaders(t *testing.T) {
	for provider, wantThrottle := range map[string]bool{providerOpenAI: true, providerOpenAICompatible: false} {
		t.Run(provider, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-ratelimit-reset-tokens", "6m0s")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error":{"type":"requests","message":"Limit reached.","code":"x"}}`)
			}))
			t.Cleanup(srv.Close)
			client, err := selectLocalBrain(ProviderConfig{Provider: provider, BaseURL: srv.URL, Model: "gpt-x"}, allCloudKeys(t))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
			if got := errors.Is(err, ErrProviderThrottled); got != wantThrottle {
				t.Errorf("throttle = %v (%v), want %v", got, err, wantThrottle)
			}
		})
	}
}
