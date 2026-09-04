// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// One writer produces every grounded draft, or the surfaces drift.
//
// Held by: this file. The claim below is not a comment asserting uniqueness
// somewhere else — it is the assertion, in both directions, and the two tests
// are what fail when it stops being true.
//
// accountdraft and persondraft each held a FULL copy of it: Write,
// writeChecked, writeWithModel, buildRequest and ParseDraft, plus a 124-line
// voicefloor.go that differed from the other only in its package line. The two
// copies differed in error-message wording and one word of one comment. Nothing
// failed while they were two, which is why they stayed two — and they were
// already deciding independently what the fence looks like, how a fenced answer
// is unwrapped, what a starved MAX_TOKENS reply does, and which drafts carry the
// Art. 50 disclosure.
//
// This gate fails in both directions:
//
//   - a drafting package that reaches the model WITHOUT the shared writer has
//     grown a second implementation, whatever it is called;
//   - the shared writer that consumes no surface has been gutted, and every
//     surface has quietly gone back to its own — the exact state this arc
//     ended, reported as clean by a gate that only looked for extras.
//
// What it deliberately does NOT judge: the prompt, the schema, the
// deterministic floor or the grounding rules. Those are per-surface by design —
// an account draft cites a dossier a contact has no equivalent of — and a gate
// demanding they converge would be asserting an answer nobody gave.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	composeRoot   = "internal/compose"
	sharedWriter  = "draftcore"
	writerPackage = "github.com/margince/margince/backend/internal/compose/draftcore"
)

// draftingPackages are the grounded drafting surfaces this gate governs.
//
// Named rather than derived, because that is the decision worth holding: a
// third grounded surface is exactly the thing that must not appear quietly, so
// adding one here is the deliberate act of widening the rule. The reply lane in
// `internal/compose` itself is not here — it is not a package of its own and it
// answers an activity rather than a record, which is a different grounding.
var draftingPackages = []string{"accountdraft", "persondraft"}

// modelCall is how a package reaches a model lane directly: the ask, and the
// request it would have to build first.
var modelCall = regexp.MustCompile(`\bai\.Ask\(|\bmodel\.Request\{`)

// Every grounded drafting surface writes through one shared writer.
//
// This test IS the holder of that claim; its other half,
// TestTheSharedWriterStillReachesTheModel below, holds the reverse direction.
func TestEveryGroundedDraftGoesThroughOneWriter(t *testing.T) {
	t.Parallel()
	for _, pkg := range draftingPackages {
		dir := filepath.Join(composeRoot, pkg)
		sources := goSourcesIn(t, dir)
		// A census that can fail short has already failed: a package this
		// cannot read is one it agrees with.
		if len(sources) == 0 {
			t.Fatalf("%s has no Go source this gate can read — either it moved or it was "+
				"deleted, and an unread package is one this gate judges nothing about", dir)
		}
		importsWriter := false
		var ownCalls []string
		for path, text := range sources {
			if strings.Contains(text, writerPackage) {
				importsWriter = true
			}
			if modelCall.MatchString(text) {
				ownCalls = append(ownCalls, path)
			}
		}
		if !importsWriter {
			t.Errorf("%s no longer imports the shared writer, so it reaches the model on its own "+
				"terms — the fence, the unwrap, the token ceiling and the disclosure are decided "+
				"there again instead of once", dir)
		}
		for _, path := range ownCalls {
			t.Errorf("%s builds its own model request or asks a lane directly. The shared writer "+
				"owns that: two implementations of one capability are two answers to one question, "+
				"and these two already drifted once", path)
		}
	}
}

// And the other direction: the shared writer still writes.
//
// Without this the gate passes over a tree where draftcore has been emptied and
// every surface has gone back to its own copy — nothing imports a writer that
// does nothing, so nothing is unauthorized, so the census reports clean about
// the exact state it exists to prevent.
func TestTheSharedWriterStillReachesTheModel(t *testing.T) {
	t.Parallel()
	sources := goSourcesIn(t, filepath.Join(composeRoot, sharedWriter))
	if len(sources) == 0 {
		t.Fatal("the shared writer package has no Go source — every drafting surface is now " +
			"writing on its own terms")
	}
	for _, text := range sources {
		if modelCall.MatchString(text) {
			return
		}
	}
	t.Error("the shared writer no longer builds a request or asks a lane, so what the drafting " +
		"surfaces import of it is a shell — each of them is reaching a model some other way")
}

// goSourcesIn reads one package's hand-written Go, by path.
//
// Tests are excluded: a test that constructs a model.Request to assert its shape
// is describing the writer rather than adding a second one.
func goSourcesIn(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		path := filepath.Join(dir, name)
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		out[path] = string(source)
	}
	return out
}
