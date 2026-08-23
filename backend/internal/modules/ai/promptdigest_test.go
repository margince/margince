// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// A digest that does not move when the wording does is worse than no digest at
// all: the surface reads as versioned, and serves yesterday's writing anyway.
func TestARewordedPromptDigestsDifferently(t *testing.T) {
	before := PromptDigest("You write a brief.")
	after := PromptDigest("You write a short brief.")
	if before == after {
		t.Errorf("rewording the prompt left the digest at %s, so a cache key would not move", before)
	}
}

// Same input, same key — otherwise every deploy rewrites every cached answer
// and the fingerprint stops meaning anything.
func TestTheSamePromptsDigestTheSameWay(t *testing.T) {
	if first, second := PromptDigest("a", "b"), PromptDigest("a", "b"); first != second {
		t.Errorf("the digest is unstable: %s then %s", first, second)
	}
}

// The separator is the whole reason this joins rather than concatenates. Two
// prompts where text shifts across the boundary are a DIFFERENT pair of
// prompts, and a digest that cannot tell them apart would hold a stale key for
// one of them — the exact failure it exists to prevent, reached by a route
// nobody would think to test for.
func TestTextMovingAcrossThePromptBoundaryIsNotTheSameInput(t *testing.T) {
	if joined, shifted := PromptDigest("ab", "c"), PromptDigest("a", "bc"); joined == shifted {
		t.Errorf("(%q,%q) and (%q,%q) share the digest %s", "ab", "c", "a", "bc", joined)
	}
}

// One prompt and two are different inputs even when the text is identical, so
// a surface that grows a second prompt gets a new key without editing either.
func TestAddingASecondPromptMovesTheDigest(t *testing.T) {
	if one, two := PromptDigest("a"), PromptDigest("a", ""); one == two {
		t.Errorf("adding an empty second prompt left the digest at %s", one)
	}
}
