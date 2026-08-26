// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

func briefLike(fence promptfence.Fence) string {
	return "You write a brief.\n" + fence.Rule("account summary")
}

// A digest that does not move when the wording does is worse than no digest at
// all: the surface reads as versioned, and serves yesterday's writing anyway.
func TestARewordedPromptDigestsDifferently(t *testing.T) {
	reworded := func(fence promptfence.Fence) string {
		return "You write a short brief.\n" + fence.Rule("account summary")
	}
	if before, after := PromptDigest(briefLike), PromptDigest(reworded); before == after {
		t.Errorf("rewording the prompt left the digest at %s, so a cache key would not move", before)
	}
}

// The boundary marker is a fresh nonce per call, so an uncanonicalized digest
// would differ on every process start and the cache would never hit — a defect
// that looks like nothing at all, just a cache that stopped earning its keep.
func TestTheSamePromptDigestsAlikeUnderDifferentBoundaries(t *testing.T) {
	if first, second := PromptDigest(briefLike), PromptDigest(briefLike); first != second {
		t.Errorf("the digest is unstable across calls: %s then %s", first, second)
	}
}

// The boundary RULE is prompt wording too. It is appended by Go code rather
// than typed into the constant, and it rides every prompt in the product — so a
// digest that covered only the constant would hold still while the text
// actually sent changed.
func TestTheBoundaryRuleRidesTheDigest(t *testing.T) {
	otherKind := func(fence promptfence.Fence) string {
		return "You write a brief.\n" + fence.Rule("deal timeline")
	}
	if same, other := PromptDigest(briefLike), PromptDigest(otherKind); same == other {
		t.Errorf("two prompts whose boundary rules differ share the digest %s", same)
	}
}

// Text moving across the boundary between two prompts is a DIFFERENT input. A
// digest that could not tell them apart would hold a stale key for one of them,
// by a route nobody would think to test for.
func TestTextMovingAcrossThePromptBoundaryIsNotTheSameInput(t *testing.T) {
	fixed := func(text string) func(promptfence.Fence) string {
		return func(promptfence.Fence) string { return text }
	}
	joined := PromptDigest(fixed("ab"), fixed("c"))
	shifted := PromptDigest(fixed("a"), fixed("bc"))
	if joined == shifted {
		t.Errorf("(%q,%q) and (%q,%q) share the digest %s", "ab", "c", "a", "bc", joined)
	}
}

// A separator only works while no prompt can contain it, and nothing enforces
// that. Under a join these two prompt sets hash alike, so one would be served
// the other's cached answer.
func TestAPromptContainingTheSeparatorDoesNotCollide(t *testing.T) {
	fixed := func(text string) func(promptfence.Fence) string {
		return func(promptfence.Fence) string { return text }
	}
	left := PromptDigest(fixed("a\x00b"), fixed("c"))
	right := PromptDigest(fixed("a"), fixed("b\x00c"))
	if left == right {
		t.Errorf("prompts differing only in where a NUL falls share the digest %s", left)
	}
}

// No prompts at all and one empty prompt are different inputs. A join maps both
// to zero bytes.
func TestNoPromptsAndOneEmptyPromptDigestDifferently(t *testing.T) {
	empty := func(promptfence.Fence) string { return "" }
	if none, one := PromptDigest(), PromptDigest(empty); none == one {
		t.Errorf("no prompts and one empty prompt share the digest %s", none)
	}
}
