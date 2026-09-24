// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Every adapter's token stream answers the question Complete answers with a
// FinishReason: did the reply finish, get cut off, or never end at all? A
// caller holding the chunks cannot tell by reading them, so the stream's END
// is the only place the answer can be — and it must mean the same on every
// wire, or the first feature that streams inherits a truncation bug on the
// providers nobody tested.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// streamEnding is how a fixture stream stops.
type streamEnding int

const (
	// endFinished is the provider's terminal for a reply that finished.
	endFinished streamEnding = iota
	// endCutOff is the provider's terminal for the output ceiling.
	endCutOff
	// endDropped is no terminal at all: the body closes mid-reply.
	endDropped
)

// streamWire is one adapter's spelling of a streamed reply: its chunks, and each
// way it can stop.
type streamWire struct {
	provider    string
	contentType string
	body        func(chunks []string, ending streamEnding) string
	// withheld is this wire's streamed spelling of a provider declining to
	// deliver the answer; empty where the wire has no such terminal.
	withheld string
	// failed is this wire's report, after the 200 went out, that generation
	// failed mid-reply.
	failed string
}

func streamWires(t *testing.T) map[string]streamWire {
	t.Helper()
	q := func(text string) string { return wireText(t, text) }
	sse := func(data string) string { return "data: " + data + "\n\n" }
	compat := func(chunks []string, ending streamEnding) string {
		var b strings.Builder
		for i, chunk := range chunks {
			finish := "null"
			// The terminal rides on the last text chunk, as a broker sends it:
			// that text must still reach the caller before the terminal does.
			if i == len(chunks)-1 && ending == endFinished {
				finish = `"stop"`
			}
			if i == len(chunks)-1 && ending == endCutOff {
				finish = `"length"`
			}
			b.WriteString(sse(`{"choices":[{"delta":{"content":` + q(chunk) + `},"finish_reason":` + finish + `}]}`))
		}
		if ending != endDropped {
			b.WriteString(sse("[DONE]"))
		}
		return b.String()
	}
	// A broker's upstream failure arrives beside the choices, or in their place.
	compatFailed := sse(`{"error":{"message":"upstream went away"},"choices":[{"delta":{"content":""},"finish_reason":"error"}]}`)
	compatWithheld := sse(`{"choices":[{"delta":{"content":""},"finish_reason":"content_filter","native_finish_reason":"SAFETY"}]}`) +
		sse("[DONE]")
	return map[string]streamWire{
		"openai": {
			provider: providerOpenAI, contentType: "text/event-stream",
			body: func(chunks []string, ending streamEnding) string {
				var b strings.Builder
				for _, chunk := range chunks {
					b.WriteString(sse(`{"type":"response.output_text.delta","delta":` + q(chunk) + `}`))
				}
				switch ending {
				case endFinished:
					b.WriteString(sse(`{"type":"response.completed","response":{"status":"completed"}}`))
				case endCutOff:
					b.WriteString(sse(`{"type":"response.incomplete","response":{"status":"incomplete",` +
						`"incomplete_details":{"reason":"max_output_tokens"}}}`))
				}
				return b.String()
			},
			withheld: sse(`{"type":"response.incomplete","response":{"status":"incomplete",` +
				`"incomplete_details":{"reason":"content_filter"}}}`),
			failed: sse(`{"type":"error","code":"server_error","message":"the server had an error"}`),
		},
		"openai_compatible": {
			provider: providerOpenAICompatible, contentType: "text/event-stream",
			body: compat, withheld: compatWithheld, failed: compatFailed,
		},
		"vllm": {
			provider: providerVLLM, contentType: "text/event-stream",
			body: compat, withheld: compatWithheld, failed: compatFailed,
		},
		"anthropic": {
			provider: providerAnthropic, contentType: "text/event-stream",
			body: func(chunks []string, ending streamEnding) string {
				return anthropicStreamBody(q, chunks, ending)
			},
			withheld: sse(`{"type":"message_delta","delta":{"stop_reason":"refusal"}}`) + sse(`{"type":"message_stop"}`),
			failed:   sse(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
		},
		"gemini": {
			provider: providerGemini, contentType: "text/event-stream",
			body: func(chunks []string, ending streamEnding) string {
				var b strings.Builder
				for i, chunk := range chunks {
					finish := ""
					// Gemini's final chunk carries both the last text and the terminal.
					if i == len(chunks)-1 && ending == endFinished {
						finish = `,"finishReason":"STOP"`
					}
					if i == len(chunks)-1 && ending == endCutOff {
						finish = `,"finishReason":"MAX_TOKENS"`
					}
					b.WriteString(sse(`{"candidates":[{"content":{"parts":[{"text":` + q(chunk) + `}]}` + finish + `}]}`))
				}
				return b.String()
			},
			withheld: sse(`{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`),
			failed:   sse(`{"error":{"status":"INTERNAL","message":"an internal error has occurred"}}`),
		},
		"ollama": {
			provider: providerOllama, contentType: "application/x-ndjson",
			body: func(chunks []string, ending streamEnding) string {
				var b strings.Builder
				for _, chunk := range chunks {
					b.WriteString(`{"model":"m","message":{"role":"assistant","content":` + q(chunk) + `},"done":false}` + "\n")
				}
				switch ending {
				case endFinished:
					b.WriteString(`{"model":"m","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}` + "\n")
				case endCutOff:
					b.WriteString(`{"model":"m","message":{"role":"assistant","content":""},"done":true,"done_reason":"length"}` + "\n")
				}
				return b.String()
			},
			failed: `{"error":"model runner has unexpectedly stopped"}` + "\n",
		},
	}
}

// anthropicStreamBody is the Messages SSE stream: the stop_reason arrives on
// message_delta, and message_stop closes the reply after it.
func anthropicStreamBody(q func(string) string, chunks []string, ending streamEnding) string {
	var b strings.Builder
	b.WriteString("data: " + `{"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":3}}}` + "\n\n")
	for _, chunk := range chunks {
		b.WriteString("data: " + `{"type":"content_block_delta","delta":{"type":"text_delta","text":` + q(chunk) + `}}` + "\n\n")
	}
	stop := map[streamEnding]string{endFinished: "end_turn", endCutOff: "max_tokens"}[ending]
	if stop != "" {
		b.WriteString("data: " + `{"type":"message_delta","delta":{"stop_reason":"` + stop + `"},"usage":{"output_tokens":5}}` + "\n\n")
		b.WriteString("data: " + `{"type":"message_stop"}` + "\n\n")
	}
	return b.String()
}

// stream opens this wire's adapter against a server that streams body.
func (w streamWire) stream(t *testing.T, body string) model.TokenStream {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Ollama asks what a model can do before it chats; the answer is not
		// what this test is about, so every model here admits to nothing.
		if r.URL.Path == "/api/show" {
			rw.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(rw, `{"capabilities":[]}`); err != nil {
				t.Errorf("writing the model description: %v", err)
			}
			return
		}
		rw.Header().Set("Content-Type", w.contentType)
		if _, err := io.WriteString(rw, body); err != nil {
			t.Errorf("writing the fixture stream: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: w.provider, BaseURL: srv.URL, Model: "m"}, allCloudKeys())
	if err != nil {
		t.Fatalf("building the %s adapter: %v", w.provider, err)
	}
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if err != nil {
		t.Fatalf("opening the %s stream: %v", w.provider, err)
	}
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing the %s stream: %v", w.provider, err)
		}
	})
	return stream
}

// drain reads a stream to its end: the text it delivered, and how it ended.
func drain(t *testing.T, stream model.TokenStream) (string, error) {
	t.Helper()
	var text strings.Builder
	for range 100 {
		chunk, ok, err := stream.Next(context.Background())
		if !ok {
			if chunk != "" {
				t.Errorf("the stream's end carried text %q, which a caller reading ok=false never sees", chunk)
			}
			return text.String(), err
		}
		if err != nil {
			t.Fatalf("a chunk arrived with an error: %v", err)
		}
		text.WriteString(chunk)
	}
	t.Fatal("the stream never ended")
	return "", nil
}

var streamedChunks = []string{`{"answer":`, `"aa`, `aa`}

func TestEveryStreamEndsCleanlyOnlyOnAFinishedReply(t *testing.T) {
	for name, wire := range streamWires(t) {
		t.Run(name, func(t *testing.T) {
			text, err := drain(t, wire.stream(t, wire.body(streamedChunks, endFinished)))
			if err != nil {
				t.Fatalf("a reply that finished on its own ended on %v", err)
			}
			if want := strings.Join(streamedChunks, ""); text != want {
				t.Errorf("text = %q, want %q", text, want)
			}
		})
	}
}

// A cut-off reply is an ANSWER, truncated: every chunk it generated reaches
// the caller, and the end says so in the port's own terms.
func TestEveryStreamReportsACutOffReplyAsTruncated(t *testing.T) {
	for name, wire := range streamWires(t) {
		t.Run(name, func(t *testing.T) {
			text, err := drain(t, wire.stream(t, wire.body(streamedChunks, endCutOff)))
			if want := strings.Join(streamedChunks, ""); text != want {
				t.Errorf("text = %q, want every chunk the provider generated and billed: %q", text, want)
			}
			if !errors.Is(err, model.ErrOutputTruncated) {
				t.Fatalf("end = %v, want model.ErrOutputTruncated — a clean end passes the half-written answer off as whole", err)
			}
			if got := finishReasonFor("", err); got != model.FinishReasonLength {
				t.Errorf("finish reason = %q, want %q, as Complete reports the same truncation", got, model.FinishReasonLength)
			}
			if errors.Is(err, model.ErrOutputWithheld) || errors.Is(err, model.ErrRequestRejected) {
				t.Errorf("a truncation also reads as another outcome: %v", err)
			}
		})
	}
}

// A body that closes before the provider's terminal is a dropped connection.
// It is neither a finished answer nor a truncated one: nothing says how much of
// the reply is missing.
func TestEveryStreamReportsADroppedConnectionAsAFailure(t *testing.T) {
	for name, wire := range streamWires(t) {
		t.Run(name, func(t *testing.T) {
			_, err := drain(t, wire.stream(t, wire.body(streamedChunks, endDropped)))
			if err == nil {
				t.Fatal("a stream that closed before its terminal ended cleanly, as though the answer were whole")
			}
			if errors.Is(err, model.ErrOutputTruncated) || errors.Is(err, model.ErrOutputWithheld) {
				t.Errorf("a dropped connection reads as a provider's terminal: %v", err)
			}
		})
	}
}

func TestEveryStreamReportsAWithheldAnswerAsWithheld(t *testing.T) {
	for name, wire := range streamWires(t) {
		if wire.withheld == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			_, err := drain(t, wire.stream(t, wire.withheld))
			if !errors.Is(err, model.ErrOutputWithheld) {
				t.Fatalf("end = %v, want model.ErrOutputWithheld", err)
			}
		})
	}
}

// A failure the provider reports after the 200 went out is neither an answer
// nor an outcome about the content: the ladder may try elsewhere.
func TestEveryStreamReportsAMidReplyFailureAsAFailure(t *testing.T) {
	for name, wire := range streamWires(t) {
		t.Run(name, func(t *testing.T) {
			if wire.failed == "" {
				t.Fatal("no mid-reply failure fixture, so nothing checks how this wire reports one")
			}
			_, err := drain(t, wire.stream(t, wire.body(streamedChunks[:1], endDropped)+wire.failed))
			if err == nil {
				t.Fatal("a stream whose provider reported a failure ended cleanly")
			}
			if errors.Is(err, model.ErrOutputTruncated) || errors.Is(err, model.ErrOutputWithheld) {
				t.Errorf("a mid-reply failure reads as a provider's terminal: %v", err)
			}
			if strings.Contains(err.Error(), "without a terminal event") {
				t.Errorf("the provider's own failure report was not read, only the missing terminal: %v", err)
			}
		})
	}
}

// streamWires is the census the tests above walk; an adapter missing from it
// goes unchecked, so this makes that a failure. The fake is exempt because its
// stream replays a script and has no wire. The withheld fixtures follow the
// same waiver the Complete parity keeps, so the two cannot disagree about which
// wires can withhold.
func TestStreamWiresCoverEveryProvider(t *testing.T) {
	covered := map[string]bool{}
	withholds := map[string]bool{}
	for _, wire := range streamWires(t) {
		covered[wire.provider] = true
		withholds[wire.provider] = withholds[wire.provider] || wire.withheld != ""
	}
	for _, provider := range knownProviders {
		if provider == ProviderFake {
			continue
		}
		if !covered[provider] {
			t.Errorf("provider %q has no row in streamWires, so nothing checks how its stream ends", provider)
		}
		if covered[provider] && !withholds[provider] && !withholdsNothing.Waived(t, provider) {
			t.Errorf("provider %q has no streamed withheld fixture and no withholdsNothing waiver", provider)
		}
	}
}

// The reader's own failure is reported only while the reply is unfinished:
// after the terminal, the answer is whole and a later read error describes
// nothing in it.
func TestAStreamReadFailureCountsOnlyBeforeTheTerminal(t *testing.T) {
	broken := fmt.Errorf("connection reset")
	unfinished := streamEnd{wire: "w"}
	if _, _, err := unfinished.outcome(broken); !errors.Is(err, broken) {
		t.Errorf("an unfinished stream's read failure = %v, want it wrapped", err)
	}
	finished := streamEnd{wire: "w"}
	finished.finish("stop")
	if _, ok, err := finished.outcome(broken); ok || err != nil {
		t.Errorf("a finished stream ended on %v %v, want a clean end", ok, err)
	}
}

// Complete over Anthropic's SSE wire reads the same stream, so the same
// mid-reply failure must reach its caller as the provider's reason, not as a
// connection that dropped.
func TestAnthropicStreamedCompleteReportsAMidReplyFailure(t *testing.T) {
	wire := streamWires(t)["anthropic"]
	handler, _ := replyWith(t, wire.contentType, wire.body(streamedChunks[:1], endDropped)+wire.failed)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: providerAnthropic, BaseURL: srv.URL, Model: "m"}, allCloudKeys())
	if err != nil {
		t.Fatalf("building the anthropic adapter: %v", err)
	}
	_, err = client.Complete(context.Background(), model.Request{
		MaxTokens: streamedCompleteThreshold + 1, Messages: []model.Message{{Role: "user", Content: "q"}},
	})
	if err == nil || !strings.Contains(err.Error(), "overloaded_error") {
		t.Fatalf("err = %v, want the provider's own reason for the failure", err)
	}
}
