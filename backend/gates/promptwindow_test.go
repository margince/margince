// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// runner.MinimumPromptWindow is the SUPPORTED FLOOR — the smallest prompt
// window any provider this build binds will carry — and this holds it equal to
// the adapter that owns the figure.
//
// TWO NUMBERS USED TO BE ONE, and separating them is what this file now checks.
// What a RUN may spend on its transcript follows the configured provider
// (Brain.PromptWindow, asked per step). What the TOOL CATALOG may grow to stays
// pinned to the floor, because the listing rides in the system prompt and is
// never elided: a catalog sized against a large cloud window is one a local
// deployment cannot serve at all, and that failure would surface only after
// somebody rebinds routing — far from the change that caused it.
//
// The floor is Ollama's, because a local runner is the tightest wire this
// product speaks to: it allocates a KV cache from the number it is handed.
// num_ctx bounds prompt and completion together, the adapter refuses to ask past
// its cap, and one completion may take the output ceiling — so the prompt gets
// what is left. "What is left" is not a subtraction, because ollamaWindowFor
// rounds a request UP by a whole bucket before the cap applies; this gate runs
// that rounding rather than restating its result.
//
// It fails in BOTH directions, and they are different bugs. A floor too HIGH
// makes the adapter ask past the cap; it is clamped, the completion is squeezed,
// and a reasoning model's reply is cut inside its thinking — a well-formed
// answer with empty content, which reads as a bad model rather than a window too
// small. A floor too LOW is the silent one: nothing breaks, the installation
// just stops using memory it already paid for, while the catalog budget derived
// from this number rations tools it no longer needs to.
//
// The subtraction alone gives 28,672, and that number is WRONG by one: an
// estimate of exactly 28,672 + 4,096 = 32,768 rounds to 36,864 and is clamped.
// A gate asserting the subtraction would have passed the broken value, which is
// why it asserts the adapter's own arithmetic instead.
//
// The constants live in two modules and most are unexported, so the gate reads
// them out of source the way the frontend parity gates read theirs. A literal
// restated here would be one more copy of the number.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
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
// The distinction is exactly where the off-by-one hides. At a floor of 28,672
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

func TestThePromptFloorLeavesRoomForItsCompletion(t *testing.T) {
	t.Parallel()
	maxContext := goConstValue(t, ollamaSource, "ollamaMaxContext")
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	completion := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")
	floor := goConstValue(t, runnerWindowSource, "MinimumPromptWindow")

	if clamped(floor+completion, bucket, maxContext) {
		t.Errorf("a prompt at MinimumPromptWindow (%d) plus a full completion (%d) is %d tokens, "+
			"which ollamaWindowFor rounds to %d — past ollamaMaxContext (%d), so the request is "+
			"clamped and the completion is squeezed. A reasoning model's reply is then cut inside "+
			"its thinking: well-formed, empty content, and it reads as a bad model rather than a "+
			"window too small. Lower the floor until the rounded request fits the cap.",
			floor, completion, floor+completion,
			((floor+completion)/bucket+1)*bucket, maxContext)
	}
}

// The floor keeps ONE BUCKET of slack, and this holds it at exactly one — too
// little and the overheads below truncate a run, too much and the catalog budget
// derived from this number rations tools for nothing.
//
// The slack exists because this package cannot see the whole prompt. Its own
// estimateTokens counts the system prompt and the message contents; the
// adapter's contextWindow also counts each message's role, an 8-byte frame per
// message, and the response schema in `Format` — several hundred tokens on a
// long transcript. Sizing to the last token arithmetic allows would put the
// real request over the cap on exactly the runs that need the room.
func TestThePromptFloorKeepsOneBucketOfSlack(t *testing.T) {
	t.Parallel()
	maxContext := goConstValue(t, ollamaSource, "ollamaMaxContext")
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	completion := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")
	floor := goConstValue(t, runnerWindowSource, "MinimumPromptWindow")

	// A prompt this much larger than the floor is what the slack absorbs; it
	// must still clear the cap. One token more than a bucket must not.
	if clamped(floor+completion+bucket-1, bucket, maxContext) {
		t.Errorf("MinimumPromptWindow %d leaves less than one %d-token bucket of slack before "+
			"ollamaMaxContext (%d). The adapter counts message roles, a per-message frame and "+
			"the response schema that this package's own estimate does not, so a run at the "+
			"floor asks for more over there than it looks like here — and the excess is "+
			"clamped, cutting the completion.", floor, bucket, maxContext)
	}
	if !clamped(floor+completion+bucket, bucket, maxContext) {
		t.Errorf("MinimumPromptWindow %d leaves MORE than one %d-token bucket of slack before "+
			"ollamaMaxContext (%d). The slack covers what this package cannot count, which is "+
			"bounded; beyond that it is window the installation allocates and never uses, and "+
			"it shrinks the catalog floor derived from this number for no reason.",
			floor, bucket, maxContext)
	}
}

// The runner's floor and the adapter's declared window are ONE number.
//
// runner.MinimumPromptWindow is what the tool-catalog budgets divide; the value
// an Ollama binding actually serves is ai.ollamaPromptWindow, derived from that
// adapter's own constants. Nothing in the type system holds the two together —
// they live in different modules and neither imports the other — so this does.
//
// The failure without it is the quiet kind. Someone raising ollamaMaxContext
// for a larger local model moves what the adapter serves and leaves the budget
// where it was; the catalog then rations tools against a limit no longer in
// force, and every gate stays green while it happens.
//
// The adapter's is an EXPRESSION, not a literal, so it is evaluated here from
// the same three constants rather than parsed. Restating the arithmetic is the
// cost of reading it from two modules; the gate below fails if the expression's
// SHAPE changes, which is what stops this restatement going stale silently.
func TestTheRunnersFloorIsTheLocalAdaptersDeclaredWindow(t *testing.T) {
	t.Parallel()
	maxContext := goConstValue(t, ollamaSource, "ollamaMaxContext")
	bucket := goConstValue(t, ollamaSource, "ollamaContextBucket")
	reserved := goConstValue(t, ollamaSource, "ollamaMaxOutputTokens")
	floor := goConstValue(t, runnerWindowSource, "MinimumPromptWindow")

	if got := maxContext - bucket - reserved; got != floor {
		t.Errorf("ai.ollamaPromptWindow computes %d (%d - %d - %d) but "+
			"runner.MinimumPromptWindow is %d.\n\n"+
			"They are one number: the first is what a local binding serves, the second is "+
			"what the tool-catalog budgets divide. Apart, the catalog rations tools against "+
			"a limit that is no longer in force, and nothing else fails to say so.",
			got, maxContext, bucket, reserved, floor)
	}

	// The restatement above is only true while the expression keeps its shape.
	// A gate that recomputed a formula the source had since changed would agree
	// with itself and describe nothing.
	source, err := os.ReadFile(ollamaSource)
	if err != nil {
		t.Fatalf("read %s: %v", ollamaSource, err)
	}
	const derivation = "const ollamaPromptWindow = ollamaMaxContext - ollamaContextBucket - ollamaMaxOutputTokens"
	if !strings.Contains(string(source), derivation) {
		t.Errorf("ollamaPromptWindow is no longer spelled %q, so the arithmetic this gate "+
			"restates may no longer be the adapter's. Update both together.", derivation)
	}
}

// The adapter reserves exactly what one completion may take.
//
// ollamaMaxOutputTokens is the completion's share of num_ctx; perCallOutputCeiling
// is what the runner will actually ask for. A reservation SMALLER than the ask
// leaves the completion clamped — the silent truncation this whole file exists
// to prevent — and a larger one spends window nothing will use.
func TestTheLocalAdapterReservesWhatOneCompletionMayTake(t *testing.T) {
	t.Parallel()
	reserved := goConstValue(t, ollamaSource, "ollamaMaxOutputTokens")
	asked := goConstValue(t, runnerWindowSource, "perCallOutputCeiling")

	if reserved != asked {
		t.Errorf("the Ollama adapter reserves %d tokens for a completion while the runner "+
			"may ask for %d.\n\n"+
			"Smaller reserves too little and the reply is cut inside a reasoning model's "+
			"thinking; larger reserves window no call will spend. They move together.",
			reserved, asked)
	}
}

// NO adapter declares a prompt window below the floor the tool catalog is
// budgeted against.
//
// runner.MinimumPromptWindow is the number the agent tool-catalog gates divide,
// and the listing they ration rides in the system prompt where nothing elides
// it. An adapter declaring LESS is one whose bindings cannot serve the catalog
// this build ships: the run would truncate on its tool list before it read a
// single observation, and nothing else would say so.
//
// The corpus is every Caps() in the ai module rather than a list written here,
// because this gate exists for the adapter somebody adds NEXT — one naming its
// own subjects would go green on exactly the addition it is meant to catch.
// vLLM is the live candidate: a local runner that sizes a KV cache the way
// Ollama does, and today declares nothing.
//
// A declaration this cannot parse is a FAILURE and never a skip. An unread line
// is how this census would quietly measure fewer adapters than the tree holds
// and still report PASS.
func TestNoAdapterDeclaresAWindowBelowTheBudgetedFloor(t *testing.T) {
	t.Parallel()
	floor := goConstValue(t, runnerWindowSource, "MinimumPromptWindow")

	declarations := adapterPromptWindows(t)
	if len(declarations) == 0 {
		t.Fatal("no adapter declares a PromptWindow, so this gate is holding nothing — " +
			"either the field was removed or the scan stopped seeing its subject")
	}
	for file, expr := range declarations {
		declared := adapterWindowValue(t, aiSource(file), expr)
		if declared < floor {
			t.Errorf("%s declares a prompt window of %d (%s), below the %d the tool catalog "+
				"is budgeted against.\n\n"+
				"The tool listing rides in the system prompt and is never elided, so a "+
				"binding on this adapter cannot serve the catalog this build ships. Either "+
				"lower runner.MinimumPromptWindow to this figure — the catalog gates then "+
				"ration against it — or raise what this adapter declares.",
				file, declared, expr, floor)
		}
	}
}

// promptWindowDeclaration matches `PromptWindow: <identifier>,` inside a Caps()
// literal. An identifier and not a number: the arithmetic belongs beside the
// adapter's own constants, and a literal here would be a second copy of it.
var promptWindowDeclaration = regexp.MustCompile(`(?m)^\s*PromptWindow:\s*([A-Za-z_][A-Za-z0-9_]*),`)

// adapterPromptWindows maps each ai-module file declaring a window to the
// constant it names.
func adapterPromptWindows(t *testing.T) map[string]string {
	t.Helper()
	const aiDir = "internal/modules/ai"
	entries, err := os.ReadDir(aiDir)
	if err != nil {
		t.Fatalf("read %s: %v", aiDir, err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(aiDir + "/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if match := promptWindowDeclaration.FindSubmatch(source); match != nil {
			out[name] = string(match[1])
		}
	}
	return out
}

// aiSource is one ai-module file, as goConstValue wants its path.
func aiSource(file string) string { return "internal/modules/ai/" + file }

// adapterWindowValue resolves the constant an adapter declares its window as.
//
// A declared window is not a literal — ollamaPromptWindow is `cap - bucket -
// reserved`, because the arithmetic belongs beside the constants it reads. So
// this evaluates the one shape those declarations take, `a - b - c` over names
// in the same file, and falls back to a plain literal for an adapter that has
// one.
//
// A form it cannot evaluate FAILS rather than passing: a window this gate
// cannot read is one it cannot hold against the floor, and reporting green on
// it would be the census-fails-short shape stated in the header.
func adapterWindowValue(t *testing.T, path, name string) int {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	pattern := regexp.MustCompile(`(?m)^const ` + regexp.QuoteMeta(name) + ` = (.+)$`)
	match := pattern.FindSubmatch(source)
	if match == nil {
		t.Fatalf("no `const %s = …` line in %s — the gate has stopped seeing its subject, "+
			"which reads as a pass; repoint it rather than deleting it", name, path)
	}
	expr := strings.TrimSpace(string(match[1]))
	if literal, err := strconv.Atoi(strings.ReplaceAll(expr, "_", "")); err == nil {
		return literal
	}
	terms := strings.Split(expr, " - ")
	if len(terms) < 2 {
		t.Fatalf("%s in %s is %q, which this gate cannot evaluate: it reads a literal or a "+
			"subtraction of named constants. Teach it the new shape rather than dropping "+
			"the adapter from the census", name, path, expr)
	}
	value := goConstValue(t, path, strings.TrimSpace(terms[0]))
	for _, term := range terms[1:] {
		value -= goConstValue(t, path, strings.TrimSpace(term))
	}
	return value
}
