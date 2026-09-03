// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The runner's prompt ceiling is DERIVED from the tightest provider it speaks
// to, and this holds the derivation it claims.
//
// PromptTokenCeiling's own comment says the number is ollamaMaxContext minus
// perCallOutputCeiling: Ollama's num_ctx bounds prompt and completion together,
// the adapter refuses to ask for more than its cap, and one completion may take
// the output ceiling. Everything left is the prompt's.
//
// Without this gate that reasoning is prose. Somebody raising the local cap for
// an unrelated reason would leave the runner's ceiling behind, and the failure
// is silent in the direction that matters: the prompt would fit num_ctx with
// room to spare, so nothing would break — the installation would simply stop
// using a window it had already paid the memory for, and the catalog floor
// derived from this number would ration tools it no longer needed to.
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

func TestThePromptCeilingIsTheLocalWindowMinusOneCompletion(t *testing.T) {
	t.Parallel()
	localCap := goConstValue(t, ollamaSource, "ollamaMaxContext")
	completion := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")
	ceiling := goConstValue(t, runnerWindowSource, "PromptTokenCeiling")

	if want := localCap - completion; ceiling != want {
		t.Errorf("PromptTokenCeiling is %d; the derivation it documents gives %d "+
			"(ollamaMaxContext %d - perCallOutputCeiling %d). Ollama's num_ctx bounds prompt AND "+
			"completion together, so a prompt ceiling above that leaves no room for the reply and "+
			"one below leaves a window the installation already allocated unused. Move this "+
			"constant with the cap, or rewrite the comment to say what the new number derives from.",
			ceiling, want, localCap, completion)
	}
}

// The ceiling has to be a whole number of the adapter's buckets, or a prompt
// that fills it rounds UP to a window past the cap — which the adapter then
// clamps, silently handing the run less room than the ceiling promised.
func TestThePromptCeilingLandsOnABucketBoundary(t *testing.T) {
	t.Parallel()
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	ceiling := goConstValue(t, runnerWindowSource, "PromptTokenCeiling")

	if ceiling%bucket != 0 {
		t.Errorf("PromptTokenCeiling %d is not a whole number of %d-token buckets (remainder %d). "+
			"ollamaWindowFor rounds a request UP to the next bucket, so a prompt at this ceiling "+
			"asks for a window past ollamaMaxContext and is clamped back — the run gets less than "+
			"this number says it may have.", ceiling, bucket, ceiling%bucket)
	}
}
