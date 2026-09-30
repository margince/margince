// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// thinkServer stands in for Ollama: it answers /api/show with show and every
// chat with a finished reply, recording what each was sent.
type thinkServer struct {
	client    model.Client
	showCalls int
	chatWire  map[string]json.RawMessage
}

func newThinkServer(t *testing.T, show string) *thinkServer {
	t.Helper()
	ts := &thinkServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := `{"message":{"content":"ok"},"done":true,"done_reason":"stop"}`
		if r.URL.Path == "/api/show" {
			ts.showCalls++
			reply = show
		} else if err := json.Unmarshal(readBody(t, r.Body), &ts.chatWire); err != nil {
			t.Errorf("chat wire not JSON: %v", err)
		}
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Errorf("writing fixture response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := SelectBrain(ProviderConfig{Provider: "ollama", Model: "m", BaseURL: srv.URL}, noCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	ts.client = client
	return ts
}

func (ts *thinkServer) complete(options map[string]json.RawMessage) error {
	_, err := ts.client.Complete(context.Background(), model.Request{
		Messages:        []model.Message{{Role: "user", Content: "hi"}},
		ProviderOptions: options,
	})
	return err
}

// What each kind of model reports about `think` is what Ollama 0.34 returned for
// the models these cases are named for; the answer must be the cheapest thing
// that model accepts. gpt-oss is the reason it is not simply `false`: given a
// boolean it answers a schema-constrained request with empty content.
func TestOllamaSendsTheCheapestThinkValueTheModelAccepts(t *testing.T) {
	cases := map[string]struct {
		show string
		want string // "" means the field is absent
	}{
		"a model that can turn thinking off (gemma4, qwen3)": {`{"thinking":{"values":[false,true],"default":true},"capabilities":["thinking"]}`, "false"},
		"a model that only grades it (gpt-oss)":              {`{"thinking":{"values":["low","medium","high"],"default":"medium"},"capabilities":["thinking"]}`, `"low"`},
		"a model that does not think":                        {`{"capabilities":["completion","tools"]}`, ""},
		"a server that does not list the choices":            {`{"capabilities":["completion","thinking"]}`, ""},
		"levels listed highest first":                        {`{"thinking":{"values":["high","medium","low"]}}`, `"low"`},
		"a level the adapter does not know":                  {`{"thinking":{"values":["deep","shallow"]}}`, `"deep"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ts := newThinkServer(t, tc.show)
			if err := ts.complete(nil); err != nil {
				t.Fatal(err)
			}
			got, present := ts.chatWire["think"]
			if tc.want == "" && present {
				t.Fatalf("think = %s, want the field absent", got)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Fatalf("think = %s (present=%v), want %s", got, present, tc.want)
			}
		})
	}
}

func TestOllamaAsksWhatAModelAcceptsOncePerModelNotOncePerCall(t *testing.T) {
	ts := newThinkServer(t, `{"thinking":{"values":[false,true]}}`)
	for range 3 {
		if err := ts.complete(nil); err != nil {
			t.Fatal(err)
		}
	}
	if ts.showCalls != 1 {
		t.Fatalf("/api/show was asked %d times for one model over three calls, want 1", ts.showCalls)
	}
}

func TestOllamaRequestOverridesTheModelsCheapestThinkValue(t *testing.T) {
	cases := map[string]struct {
		options map[string]json.RawMessage
		want    string
	}{
		"think on":     {map[string]json.RawMessage{"ollama": json.RawMessage(`{"think":true}`)}, "true"},
		"effort level": {map[string]json.RawMessage{"ollama": json.RawMessage(`{"think":"high"}`)}, `"high"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ts := newThinkServer(t, `{"thinking":{"values":[false,true]}}`)
			if err := ts.complete(tc.options); err != nil {
				t.Fatal(err)
			}
			if got := string(ts.chatWire["think"]); got != tc.want {
				t.Fatalf("think = %s, want %s", got, tc.want)
			}
			if ts.showCalls != 0 {
				t.Fatalf("a request that names its own value still asked /api/show %d times", ts.showCalls)
			}
		})
	}
}

func TestOllamaRefusesAThinkValueThatIsNeitherABooleanNorALevel(t *testing.T) {
	for _, bad := range []string{`{"think":5}`, `{"think":{"level":"low"}}`, `{"think":["low"]}`} {
		t.Run(bad, func(t *testing.T) {
			ts := newThinkServer(t, `{}`)
			err := ts.complete(map[string]json.RawMessage{"ollama": json.RawMessage(bad)})
			if !errors.Is(err, errOllamaThinkShape) {
				t.Fatalf("options %s: got %v, want errOllamaThinkShape — a shape the server cannot read must fail here, not as an HTTP 400 mid-run", bad, err)
			}
		})
	}
}

// A model the server does not have is the one /api/show failure that must
// surface, and it must be named as the show that failed: chat would fail too, but
// on a message that reads as a problem with the call rather than the model.
func TestOllamaNamesTheModelTheServerDoesNotHave(t *testing.T) {
	var chatCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			http.Error(w, `{"error":"model 'nope' not found"}`, http.StatusNotFound)
			return
		}
		chatCalls++
		if _, err := w.Write([]byte(`{"message":{"content":"ok"},"done":true}`)); err != nil {
			t.Errorf("writing fixture response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := SelectBrain(ProviderConfig{Provider: "ollama", Model: "nope", BaseURL: srv.URL}, noCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), `describing model "nope"`) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("got %v, want the failure named as describing the model, with the server's reason", err)
	}
	if chatCalls != 0 {
		t.Fatalf("chat was called %d times after the model was reported missing", chatCalls)
	}
}

// A proxy in front of Ollama, or a server that predates /api/show, has no such
// route, or refuses it (401/403) while letting chat through. Chat worked through
// it before the field existed, so it must still work.
func TestOllamaStillChatsThroughAServerWithNoShowEndpoint(t *testing.T) {
	for name, status := range map[string]int{"404 page not found": http.StatusNotFound, "405": http.StatusMethodNotAllowed, "501": http.StatusNotImplemented, "401": http.StatusUnauthorized, "403": http.StatusForbidden} {
		t.Run(name, func(t *testing.T) {
			var wire map[string]json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/show" {
					http.Error(w, "404 page not found", status)
					return
				}
				if err := json.Unmarshal(readBody(t, r.Body), &wire); err != nil {
					t.Errorf("chat wire not JSON: %v", err)
				}
				if _, err := w.Write([]byte(`{"message":{"content":"ok"},"done":true}`)); err != nil {
					t.Errorf("writing fixture response: %v", err)
				}
			}))
			t.Cleanup(srv.Close)
			client, err := SelectBrain(ProviderConfig{Provider: "ollama", Model: "m", BaseURL: srv.URL}, noCloudKeys())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatalf("chat failed because /api/show was missing: %v", err)
			}
			if got, present := wire["think"]; present {
				t.Fatalf("think = %s, want the field absent when the model cannot be described", got)
			}
		})
	}
}

// A completion cut at num_predict is the case structured.go retries with
// "answer more briefly" instead of a schema complaint — but only when the
// adapter reports it. On a model that thinks, this is also how an answer that
// never began (empty content) is told apart from one that finished empty.
func TestOllamaReportsACutOffCompletionAsTruncatedAndAFinishedOneAsNot(t *testing.T) {
	for _, doneReason := range []string{"length", "stop"} {
		t.Run(doneReason, func(t *testing.T) {
			client := newOllamaForTest(t, func(w http.ResponseWriter, _ *http.Request) {
				body := `{"message":{"content":""},"done":true,"done_reason":"` + doneReason + `"}`
				if _, err := w.Write([]byte(body)); err != nil {
					t.Errorf("writing fixture response: %v", err)
				}
			})
			resp, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := resp.FinishReason == model.FinishReasonLength, doneReason == "length"; got != want {
				t.Fatalf("done_reason %q: truncated=%v, want %v (FinishReason %q)", doneReason, got, want, resp.FinishReason)
			}
		})
	}
}
