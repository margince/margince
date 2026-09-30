// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// pricingPageStub stands in for the web read: the refresh's own fetcher rightly
// refuses a loopback test server, and what a site is certified on is the page
// text, never the retrieval of it.
type pricingPageStub struct{ text string }

func (f pricingPageStub) Fetch(_ context.Context, _ string) (webread.Doc, error) {
	return webread.Doc{Text: f.text}, nil
}

// assertSameRateExtractRequest compares two requests for the same page. The fence
// marker is minted per call, so it is normalised away — every other byte of a
// request the certification lane claims production sends must match the one
// production sent.
func assertSameRateExtractRequest(t *testing.T, production, certified model.Request) {
	t.Helper()
	normalize := func(req model.Request) model.Request {
		marker, declared := promptfence.MarkerIn(req.System)
		if !declared {
			t.Fatalf("the request declares no data boundary: %q", req.System)
		}
		out := req
		out.System = strings.ReplaceAll(req.System, marker, "MARKER")
		out.Messages = make([]model.Message, len(req.Messages))
		for i, message := range req.Messages {
			out.Messages[i] = model.Message{
				Role:    message.Role,
				Content: strings.ReplaceAll(message.Content, marker, "MARKER"),
			}
		}
		return out
	}
	production, certified = normalize(production), normalize(certified)
	if production.System != certified.System {
		t.Errorf("the certified system prompt is not production's:\n%q\n%q", certified.System, production.System)
	}
	if len(production.Messages) != len(certified.Messages) {
		t.Fatalf("the certified request carries %d turns, production sent %d",
			len(certified.Messages), len(production.Messages))
	}
	for i, message := range production.Messages {
		if certified.Messages[i] != message {
			t.Errorf("certified turn %d = %+v, production sent %+v", i, certified.Messages[i], message)
		}
	}
	if certified.MaxTokens != production.MaxTokens ||
		string(certified.ResponseSchema) != string(production.ResponseSchema) ||
		certified.SecretStripper == nil {
		t.Errorf("the certified request lost the governed bounds production sends: %+v", certified)
	}
}

// numberPassages must turn a real (messy, free-tier-interleaved) page into cited
// passages the rate_extract task grounds against.
func TestNumberPassagesOnRealGeminiSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/gemini_pricing_reduced.txt")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	numbered := numberPassages(string(raw))
	if !strings.HasPrefix(numbered, "[s0] ") {
		t.Fatalf("numbered text does not start with a passage id: %.40q", numbered)
	}
	if !strings.Contains(numbered, "$1.50") {
		t.Error("expected the captured input price $1.50 to survive numbering")
	}
	if strings.Contains(numbered, "\n\n") {
		t.Error("numberPassages left a blank line (empty lines must be dropped)")
	}
}

// The probe reports a passage count in two places and production numbers
// passages in a third. This holds the shared counter to the numbering it
// describes, so neither can drift into reporting a different rule.
func TestCountPassagesAgreesWithTheNumbering(t *testing.T) {
	for _, body := range []string{
		"",
		"one line",
		"a\nb\nc",
		"a\n\n   \nb\n",
		`{"data":[{"id":"a"},{"id":"b"}]}`,
		"trailing\n",
		"\n\nleading",
	} {
		numbered := numberPassages(body)
		want := strings.Count(numbered, "\n")
		if got := CountPassages(body); got != want {
			t.Errorf("CountPassages(%q) = %d, but numberPassages emitted %d passages", body, got, want)
		}
	}
}
