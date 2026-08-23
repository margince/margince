// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PromptDigest fingerprints the prompts a surface sends, so a cache key moves
// when the wording does.
//
// It exists because the alternative is a hand-typed constant somebody has to
// remember to bump, and that does not hold. A surface's own docblock recorded
// the failure in its own words: a deploy reworded the text, left the constant
// alone, and kept serving the old sentences to every record whose facts had not
// moved — which is most of them. Nothing errors, nothing is logged, and the
// reader has no way to tell they are looking at yesterday's writing.
//
// WHAT IT CANNOT COVER, so a caller does not mistake it for the whole answer:
// wording built by Go code — a `fmt.Sprintf` in a deterministic floor — is not
// a string this can hash. A surface with such a floor keeps an explicit
// constant for that half, and the two ride the fingerprint together.
//
// NOT to be confused with `aicert.PromptVersion`, which answers a different
// question: whether a stored certification record still describes the request
// the tree builds today. That one hashes the BUILT REQUEST through the task
// census, because a cert record has to notice a change anywhere in assembly.
// This one hashes the prompt text, because a cache key only has to move when
// the wording does.
//
// The prefix keeps the value readable in a stored row. A fingerprint that
// changed for a reason nobody can name is worse than one that is merely long,
// and eight bytes is ample for a cache key that only has to differ.
func PromptDigest(prompts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(prompts, "\x00")))
	return "prompts-" + hex.EncodeToString(sum[:8])
}
