// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The runner's prompt ceiling is DERIVED from the tightest provider it speaks
// to, and this holds the derivation it claims.
//
// Ollama's num_ctx bounds prompt and completion together, the adapter refuses
// to ask for more than its cap, and one completion may take the output ceiling.
// So the prompt gets what is left — but "what is left" is not a subtraction,
// because ollamaWindowFor rounds a request UP by a whole bucket before the cap
// is applied. This gate runs that rounding rather than restating its result.
//
// It fails in BOTH directions, and they are different bugs. A ceiling too HIGH
// makes the adapter ask for a window past the cap; it is clamped back, so the
// completion is squeezed and a reasoning model's reply is cut inside its
// thinking — a well-formed answer with empty content, which reads as a bad
// model rather than a window too small. A ceiling too LOW is the silent one:
// nothing breaks, the installation just stops using a window it already paid
// the memory for, while the catalog floor derived from this number rations
// tools it no longer needs to.
//
// The subtraction alone gives 28,672 and that number is WRONG by one: an
// estimate of exactly 28,672 + 4,096 = 32,768 rounds to 36,864 and is clamped.
// A gate that asserted the subtraction would have passed the broken value,
// which is why it asserts the adapter's own arithmetic instead.
//
// The three constants live in two modules and none is exported, so the gate
// reads them out of the source the way the frontend parity gates read theirs.
// A literal restated here would be a fourth copy of the number.

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

const (
	runnerWindowSource = "internal/modules/agents/runner/window.go"
	ollamaSource       = "internal/modules/ai/ollama.go"
)

// goConstValue reads one `const <name> = <digits>` out of a Go source file,
// tolerating the underscore separators the tree writes large numbers with.
func goConstValue(t *testing.T, path, name string) int {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// Anchored to a line start so a mention inside a comment cannot answer for
	// the declaration — the whole point is to read what COMPILES, and both
	// names appear in prose in these same files.
	pattern := regexp.MustCompile(`(?m)^const ` + regexp.QuoteMeta(name) + ` = ([0-9_]+)`)
	match := pattern.FindSubmatch(source)
	if match == nil {
		t.Fatalf("no `const %s = <number>` line in %s — the gate has stopped seeing its subject, "+
			"which reads as a pass; repoint it rather than deleting it", name, path)
	}
	digits := ""
	for _, r := range string(match[1]) {
		if r != '_' {
			digits += string(r)
		}
	}
	value, err := strconv.Atoi(digits)
	if err != nil {
		t.Fatalf("const %s in %s is not a number: %v", name, path, err)
	}
	return value
}

// clamped reports whether ollamaWindowFor would have to cut this request down
// to the cap — which is the real question, and not the one a comparison of the
// answer against the estimate asks.
//
// The distinction is exactly where the off-by-one hides. At a ceiling of 28,672
// the request rounds to 36,864 and is clamped to 32,768, and 32,768 still
// happens to equal the estimate — so "did the answer cover the estimate?"
// reads as yes for a value that truncates. What went wrong is the clamp, so the
// clamp is what this asks about.
//
// The rounding formula is copied once here because the adapter's own is
// unexported in another module; the constants it rounds with are read from
// that module rather than written down, so a change to any of them moves this
// gate with it.
func clamped(estimate, bucket, maxContext int) bool {
	return (estimate/bucket+1)*bucket > maxContext
}

func TestAPromptAtTheCeilingLeavesRoomForItsCompletion(t *testing.T) {
	t.Parallel()
	maxContext := goConstValue(t, ollamaSource, "ollamaMaxContext")
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	completion := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")
	ceiling := goConstValue(t, runnerWindowSource, "PromptTokenCeiling")

	if clamped(ceiling+completion, bucket, maxContext) {
		t.Errorf("a prompt at PromptTokenCeiling (%d) plus a full completion (%d) is %d tokens, "+
			"which ollamaWindowFor rounds to %d — past ollamaMaxContext (%d), so the request is "+
			"clamped and the completion is squeezed. A reasoning model's reply is then cut inside "+
			"its thinking: well-formed, empty content, and it reads as a bad model rather than a "+
			"window too small. Lower the ceiling until the rounded request fits the cap.",
			ceiling, completion, ceiling+completion,
			((ceiling+completion)/bucket+1)*bucket, maxContext)
	}
}

// The ceiling keeps ONE BUCKET of slack, and this holds it at exactly one — too
// little and the overheads below truncate a run, too much and the catalog floor
// derived from this number rations tools for nothing.
//
// The slack exists because this package cannot see the whole prompt. Its own
// estimateTokens counts the system prompt and the message contents; the
// adapter's contextWindow also counts each message's role, an 8-byte frame per
// message, and the response schema in `Format` — several hundred tokens on a
// long transcript. Sizing to the last token arithmetic allows would put the
// real request over the cap on exactly the runs that need the room.
func TestThePromptCeilingKeepsOneBucketOfSlack(t *testing.T) {
	t.Parallel()
	maxContext := goConstValue(t, ollamaSource, "ollamaMaxContext")
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	completion := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")
	ceiling := goConstValue(t, runnerWindowSource, "PromptTokenCeiling")

	// A prompt this much larger than the ceiling is what the slack absorbs; it
	// must still clear the cap. One token more than a bucket must not.
	if clamped(ceiling+completion+bucket-1, bucket, maxContext) {
		t.Errorf("PromptTokenCeiling %d leaves less than one %d-token bucket of slack before "+
			"ollamaMaxContext (%d). The adapter counts message roles, a per-message frame and "+
			"the response schema that this package's own estimate does not, so a run at the "+
			"ceiling asks for more over there than it looks like here — and the excess is "+
			"clamped, cutting the completion.", ceiling, bucket, maxContext)
	}
	if !clamped(ceiling+completion+bucket, bucket, maxContext) {
		t.Errorf("PromptTokenCeiling %d leaves MORE than one %d-token bucket of slack before "+
			"ollamaMaxContext (%d). The slack covers what this package cannot count, which is "+
			"bounded; beyond that it is window the installation allocates and never uses, and "+
			"it shrinks the catalog floor derived from this number for no reason.",
			ceiling, bucket, maxContext)
	}
}
