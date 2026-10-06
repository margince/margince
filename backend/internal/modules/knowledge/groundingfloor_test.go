// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestTheFloorFollowsTheModelNotTheProviderOrWidth(t *testing.T) {
	for _, identity := range []string{
		"gemini/gemini-embedding-001@1536",
		"gemini_vertex/gemini-embedding-001@768",
	} {
		if got := BindingFloor(identity); got != 0.65 {
			t.Errorf("BindingFloor(%q) = %v, want 0.65", identity, got)
		}
	}
}

func TestABindingWithoutASeparatingFloorHasNone(t *testing.T) {
	for _, identity := range []string{
		"openai_compatible/mistralai/mistral-embed-2312@1024", // bands overlap
		"ollama/bge-m3@1024", // never measured
		"",
	} {
		if got := BindingFloor(identity); got != 0 {
			t.Errorf("BindingFloor(%q) = %v, want no floor", identity, got)
		}
	}
}

func TestAnOverrideWinsEvenBelowTheBindingFloor(t *testing.T) {
	low, zero := 0.2, 0.0
	gemini := "gemini/gemini-embedding-001@1536"
	if got := EffectiveFloor(&low, gemini); got != 0.2 {
		t.Errorf("override 0.2 = %v", got)
	}
	if got := EffectiveFloor(&zero, gemini); got != 0 {
		t.Errorf("an override of 0 is a choice, not an absence: got %v", got)
	}
	if got := EffectiveFloor(nil, gemini); got != 0.65 {
		t.Errorf("no override = %v, want the binding's 0.65", got)
	}
}

func TestStartupWarnsWhereAGroundingFloorCannotBeEnforced(t *testing.T) {
	cases := []struct {
		identity string
		want     string
	}{
		{"openai_compatible/mistralai/mistral-embed-2312@1024", "no floor separates covered from uncovered questions"},
		{"ollama/bge-m3@1024", "no measured grounding floor"},
		{"gemini/gemini-embedding-001@1536", ""},
		{"", ""},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		WarnIfUngated(slog.New(slog.NewTextHandler(&buf, nil)), c.identity)
		if c.want == "" && buf.Len() != 0 {
			t.Errorf("%q warned: %s", c.identity, buf.String())
		}
		if c.want != "" && !strings.Contains(buf.String(), c.want) {
			t.Errorf("%q: log %q lacks %q", c.identity, buf.String(), c.want)
		}
	}
}
