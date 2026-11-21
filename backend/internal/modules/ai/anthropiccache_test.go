// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// anthropicCacheWire is the part of a sent request that says what may be cached.
type anthropicCacheWire struct {
	System []struct {
		Text         string                 `json:"text"`
		CacheControl *anthropicCacheControl `json:"cache_control"`
	} `json:"system"`
	Tools []struct {
		Name         string                 `json:"name"`
		CacheControl *anthropicCacheControl `json:"cache_control"`
	} `json:"tools"`
}

// sendForCache sends req through the adapter and returns what reached the wire.
func sendForCache(t *testing.T, req model.Request) anthropicCacheWire {
	t.Helper()
	var sent []byte
	client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
		sent = readBody(t, r.Body)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "ok"}},
		}); err != nil {
			t.Errorf("encoding fixture response: %v", err)
		}
	})
	req.Messages = []model.Message{{Role: roleUser, Content: "hello"}}
	if _, err := client.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var wire anthropicCacheWire
	if err := json.Unmarshal(sent, &wire); err != nil {
		t.Fatalf("wire not JSON: %v (%s)", err, sent)
	}
	return wire
}

// longRules is fixed instruction text over the smallest cacheable size.
var longRules = strings.Repeat("Label each message by what it asks of us. ", 60) + "\n\n"

func TestAnthropicMarksTheStableRulesAndNotTheBoundarySentence(t *testing.T) {
	fence := promptfence.New()
	wire := sendForCache(t, model.Request{System: longRules + fence.Rule("message")})
	if len(wire.System) != 2 {
		t.Fatalf("want the rules and the boundary sentence as two blocks, got %d", len(wire.System))
	}
	rules, boundary := wire.System[0], wire.System[1]
	if rules.Text != longRules || rules.CacheControl == nil || rules.CacheControl.Type != "ephemeral" {
		t.Errorf("the stable rules went as %q with marker %v; want them marked ephemeral", rules.Text, rules.CacheControl)
	}
	if boundary.CacheControl != nil || boundary.Text != fence.Rule("message") {
		t.Errorf("the boundary sentence went as %q with marker %v; want it whole and unmarked", boundary.Text, boundary.CacheControl)
	}
}

func TestAnthropicMarksNoPrefixUnderTheSmallestCacheableSize(t *testing.T) {
	system := "Be brief.\n" + promptfence.New().Rule("message")
	wire := sendForCache(t, model.Request{System: system})
	if len(wire.System) != 1 || wire.System[0].CacheControl != nil || wire.System[0].Text != system {
		t.Errorf("a short system prompt went as %+v; want one unmarked block", wire.System)
	}
}

// Two calls of one prompt differ only in their nonce. The block that is marked
// must be byte-identical, or the provider never reads what the first one wrote.
func TestAnthropicMarkedRulesAreTheSameUnderEveryNonce(t *testing.T) {
	first := sendForCache(t, model.Request{System: longRules + promptfence.New().Rule("message")})
	second := sendForCache(t, model.Request{System: longRules + promptfence.New().Rule("message")})
	if first.System[0].Text != second.System[0].Text {
		t.Error("the marked rules differ between two calls of the same prompt")
	}
	if first.System[1].Text == second.System[1].Text {
		t.Error("two calls carried the same boundary sentence; the nonce is per call")
	}
}

func TestAnthropicMarksAWholeSystemPromptThatDeclaresNoBoundary(t *testing.T) {
	wire := sendForCache(t, model.Request{System: longRules})
	if len(wire.System) != 1 || wire.System[0].CacheControl == nil {
		t.Errorf("a long prompt with no captured text went as %+v; want it marked whole", wire.System)
	}
}

func TestAnthropicMarksTheLastToolOnly(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	wire := sendForCache(t, model.Request{
		System: "Be brief.",
		Tools: []model.ToolDef{
			{Name: "find_records", InputSchema: schema},
			{Name: "read_record", InputSchema: schema},
		},
	})
	if len(wire.Tools) != 2 || wire.Tools[0].CacheControl != nil || wire.Tools[1].CacheControl == nil {
		t.Errorf("tools went as %+v; want the marker on the last one only", wire.Tools)
	}
}
