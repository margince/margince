// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every optional seam the capture Sink offers is wired by the composition root.
//
// A Sink seam is a nil-able function field: a sink without it captures, just
// with one behaviour missing. That is what makes the seams useful and also what
// makes them silent — nothing fails when compose forgets one, the feature is
// simply absent in production.
//
// The integration tests cannot see it. Each builds its own sink with the seams
// it cares about (compose/importthencapture_integration_test.go and four
// others), so the deduplication, the take-over and the meeting cancellation all
// pass their tests while newCaptureSink injects none of them. Verified by
// mutation: deleting WithMessageIdentity from newCaptureSink leaves every test
// in that file green.
//
// So the corpus is DERIVED from the Sink's own With* surface rather than listed
// here. A seam added tomorrow enrolls itself, which is the only version of this
// check that cannot fall short as the sink grows.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// sinkSeamBuilder matches a `func (s *Sink) WithX(...)` declaration — the
// builder shape every optional seam is offered through.
var sinkSeamBuilder = regexp.MustCompile(`func \(s \*Sink\) (With[A-Za-z0-9_]+)\(`)

func TestEveryCaptureSinkSeamIsWiredByCompose(t *testing.T) {
	t.Parallel()
	seams := captureSinkSeams(t)
	// The floor is the census's own proof of life: a scan that stopped finding
	// builders would otherwise report PASS over an empty corpus.
	if len(seams) < 4 {
		t.Fatalf("found %d Sink seam builders — the scan for them has stopped working", len(seams))
	}
	wiring := readGateFile(t, "internal/compose/capture.go")
	var missing []string
	for _, seam := range seams {
		// The builders chain across lines, so the call begins a line after
		// leading whitespace rather than following a dot on the same one.
		if !strings.Contains(wiring, seam+"(") {
			missing = append(missing, seam)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("capture.Sink seams offered but never wired in compose/capture.go: %s\n"+
			"A nil seam is a feature absent in production that every integration "+
			"test still passes, because each builds its own sink. Wire it in "+
			"newCaptureSink, or delete the builder if nothing should use it.",
			strings.Join(missing, ", "))
	}
}

// captureSinkSeams reads the seam builders off the capture package itself.
func captureSinkSeams(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, path := range goSourceFiles(t, ".") {
		if !strings.HasPrefix(path, "internal/modules/capture/") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, m := range sinkSeamBuilder.FindAllStringSubmatch(readGateFile(t, path), -1) {
			found = append(found, m[1])
		}
	}
	sort.Strings(found)
	return found
}

func readGateFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}
