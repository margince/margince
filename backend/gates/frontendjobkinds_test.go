// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// System health names every declared job kind in words, with the kind under it.
//
// The names live in frontend/src/format/jobkinds.ts. A declared kind missing
// there shows an operator a bare key. A name for a kind no longer declared says
// the list is not maintained. So both directions fail.

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

var frontendJobKinds = filepath.Join(repoRoot, "frontend", "src", "format", "jobkinds.ts")

var tsCommentInJobKinds = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// tsJobKindEntry is one line of the literal: the kind and the catalog key that names it.
var tsJobKindEntry = regexp.MustCompile(`^"?([a-z0-9_]+)"?\s*:\s*"jobs\.kind\.([a-z0-9_]+)",?$`)

// jobKindCensusFloor sits well under what the contract declares, so retiring a
// few kinds passes and a read that finds almost nothing fails loudly.
const jobKindCensusFloor = 45

func TestEveryDeclaredJobKindHasAFrontendName(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for kind := range jobs.Declared() {
		declared[kind] = true
	}
	named := tsJobKindNames(t)
	if len(declared) < jobKindCensusFloor || len(named) < jobKindCensusFloor {
		t.Fatalf("read %d declared kinds and %d named ones, expected at least %d of each: a census that reads almost nothing agrees with everything",
			len(declared), len(named), jobKindCensusFloor)
	}
	for _, drift := range jobKindNameDrift(declared, named) {
		t.Error(drift)
	}
}

// The comparison over planted sets, so each direction is seen to fail.
func TestJobKindNameDriftFailsInBothDirections(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{"capture_sync": true, "gmail_sync": true}
	named := map[string]bool{"capture_sync": true, "retired_pass": true}
	drift := jobKindNameDrift(declared, named)
	if len(drift) != 2 ||
		!strings.Contains(drift[0], `"gmail_sync"`) || !strings.Contains(drift[0], "has no name") ||
		!strings.Contains(drift[1], `"retired_pass"`) || !strings.Contains(drift[1], "no longer declares") {
		t.Fatalf("a missing name and a dead one must each fail, got %q", drift)
	}
	if got := jobKindNameDrift(declared, declared); len(got) != 0 {
		t.Fatalf("matching sets must agree, got %q", got)
	}
}

func jobKindNameDrift(declared, named map[string]bool) []string {
	var drift []string
	for _, kind := range slices.Sorted(maps.Keys(declared)) {
		if !named[kind] {
			drift = append(drift, fmt.Sprintf("api/jobs.yaml declares %q and it has no name in %s (add jobs.kind.%s to en.ts, de.ts and vi.ts), so System health shows the bare key", kind, frontendJobKinds, kind))
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(named)) {
		if !declared[kind] {
			drift = append(drift, fmt.Sprintf("%s names %q, which api/jobs.yaml no longer declares: delete the entry and its jobs.kind.%s messages", frontendJobKinds, kind, kind))
		}
	}
	return drift
}

// tsJobKindNames reads the JOB_KIND_NAME literal one entry per line. A line it
// cannot read fails, because an entry the parser skips is one the gate agrees with.
func tsJobKindNames(t *testing.T) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(frontendJobKinds)
	if err != nil {
		t.Fatalf("reading the frontend job kind names: %v", err)
	}
	const opener = "const JOB_KIND_NAME: Readonly<Record<string, MessageKey>> = {"
	_, rest, opened := strings.Cut(string(source), opener)
	if !opened {
		t.Fatalf("%s no longer declares %s: this gate is reading a shape that is gone", frontendJobKinds, opener)
	}
	body, _, closed := strings.Cut(rest, "\n};")
	if !closed {
		t.Fatalf("%s: JOB_KIND_NAME is not closed by `};` on its own line", frontendJobKinds)
	}
	named := map[string]bool{}
	for line := range strings.SplitSeq(tsCommentInJobKinds.ReplaceAllString(body, ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := tsJobKindEntry.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("%s: JOB_KIND_NAME holds %q, which is not a `kind: \"jobs.kind.kind\"` entry this gate can read", frontendJobKinds, line)
		}
		if match[1] != match[2] {
			t.Errorf("%s names %q with jobs.kind.%s: each kind keys its own message", frontendJobKinds, match[1], match[2])
		}
		named[match[1]] = true
	}
	return named
}
