// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/promptfence"
)

// PromptDigest fingerprints the system prompts a surface SENDS, so a cache key
// moves when the wording does.
//
// A cached answer is keyed by a fingerprint. If the fingerprint does not move
// when the prompt does, a deploy that rewords one keeps serving text written
// the old way to every record whose facts have not changed — which is most of
// them. Nothing errors and nothing is logged, so the reader has no way to tell.
//
// It takes the BUILDERS rather than the prompt constants, because a constant is
// not what reaches the model: each surface appends its data-boundary rule to
// one, and that sentence rides every prompt in the product. A digest over the
// constants alone holds still while the text actually sent changes, which is
// the same defect reached by a different route.
//
// The boundary's marker is a fresh nonce per call, so a built prompt is
// canonicalized before hashing. Without that the digest would differ on every
// process start and the cache would never hit.
//
// WHAT IT CANNOT COVER, so a caller does not mistake it for the whole answer:
// wording built by Go code from runtime facts — a `fmt.Sprintf` in a
// deterministic floor — is not reachable from a builder that takes only a
// fence. A surface with such a floor keeps an explicit constant for that half,
// and the two ride the fingerprint together.
//
// NOT to be confused with `aicert.PromptVersion`, which answers a different
// question: whether a stored certification record still describes the request
// the tree builds today. That one hashes the whole built REQUEST through the
// task census, because a cert record has to notice a change anywhere in
// assembly. This one hashes the prompt, because a cache key only has to move
// when the wording does.
//
// Each prompt is length-prefixed rather than joined by a separator. A separator
// only works while no prompt can contain it, and nothing enforces that: under a
// join, ("a\x00b", "c") and ("a", "b\x00c") hash alike, as do no prompts at all
// and one empty one. Two different prompt sets sharing a key is the stale-answer
// defect this exists to prevent, reached from the inside.
//
// The prefix keeps the value readable in a stored row, and eight bytes is ample
// for a key that only has to differ.
func PromptDigest(build ...func(promptfence.Fence) string) string {
	fence := promptfence.New()
	var framed []byte
	for _, buildPrompt := range build {
		prompt := buildPrompt(fence)
		canonical := promptfence.Canonicalize(prompt, prompt)
		framed = binary.AppendUvarint(framed, uint64(len(canonical)))
		framed = append(framed, canonical...)
	}
	sum := sha256.Sum256(framed)
	return "prompts-" + hex.EncodeToString(sum[:8])
}
