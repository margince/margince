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

// tsJobKindEntry is one line of the literal: the kind and the catalog key that
// names it. That key is its own, or the label of the principal it runs as.
var tsJobKindEntry = regexp.MustCompile(`^"?([a-z0-9_]+)"?\s*:\s*"(?:jobs\.kind|systemJob)\.([a-z0-9_]+)",?$`)

// jobKindCensusFloor sits well under what the contract declares, so retiring a
// few kinds passes and a read that finds almost nothing fails loudly.
const jobKindCensusFloor = 45

func TestEveryDeclaredJobKindHasAFrontendName(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for kind := range jobs.Declared() {
		if !jobs.IsExtensionKind(kind) {
			declared[kind] = true
		}
	}
	source, err := os.ReadFile(frontendJobKinds)
	if err != nil {
		t.Fatalf("reading the frontend job kind names: %v", err)
	}
	named, problems := tsJobKindNames(string(source))
	for _, problem := range problems {
		t.Error(problem)
	}
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
// cannot read is a problem, because an entry the parser skips is one the gate agrees with.
func tsJobKindNames(source string) (map[string]bool, []string) {
	const opener = "const JOB_KIND_NAME: Readonly<Record<string, MessageKey>> = {"
	_, rest, opened := strings.Cut(source, opener)
	if !opened {
		return nil, []string{fmt.Sprintf("%s no longer declares %s: this gate is reading a shape that is gone", frontendJobKinds, opener)}
	}
	body, _, closed := strings.Cut(rest, "\n};")
	if !closed {
		return nil, []string{fmt.Sprintf("%s: JOB_KIND_NAME is not closed by `};` on its own line", frontendJobKinds)}
	}
	named := map[string]bool{}
	var problems []string
	for line := range strings.SplitSeq(tsCommentInJobKinds.ReplaceAllString(body, ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := tsJobKindEntry.FindStringSubmatch(line)
		switch {
		case match == nil:
			problems = append(problems, fmt.Sprintf("%s: JOB_KIND_NAME holds %q, which is not a `kind: \"jobs.kind.kind\"` or `kind: \"systemJob.kind\"` entry this gate can read", frontendJobKinds, line))
		case match[1] != match[2]:
			problems = append(problems, fmt.Sprintf("%s names %q with the message of %q: each kind keys a message of its own name", frontendJobKinds, match[1], match[2]))
		default:
			named[match[1]] = true
		}
	}
	return named, problems
}

// Each shape the parser could misread, planted. It reads it right or says so.
func TestTheJobKindParserReadsOrRefusesEveryShape(t *testing.T) {
	t.Parallel()
	const opener = "const JOB_KIND_NAME: Readonly<Record<string, MessageKey>> = {\n"
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"a comment line":                      {"  // a note\n  a: \"jobs.kind.a\",\n};", []string{"a"}},
		"a comment holding };":                {"  // ends at };\n  a: \"jobs.kind.a\",\n};", []string{"a"}},
		"a block comment holding };":          {"  a: \"jobs.kind.a\",\n  /* ends\n};\n  */\n  b: \"jobs.kind.b\",\n};", nil},
		"a multi-line entry":                  {"  a:\n    \"jobs.kind.a\",\n};", nil},
		"a spread":                            {"  ...OTHER,\n  a: \"jobs.kind.a\",\n};", nil},
		"single quotes":                       {"  a: 'jobs.kind.a',\n};", nil},
		"two entries on one line":             {"  a: \"jobs.kind.a\", b: \"jobs.kind.b\",\n};", nil},
		"a key naming another kind's message": {"  a: \"jobs.kind.b\",\n};", nil},
		"a namesake principal's label":        {"  a: \"systemJob.a\",\n  b: \"jobs.kind.b\",\n};", []string{"a", "b"}},
		"another principal's label":           {"  a: \"systemJob.b\",\n};", nil},
	} {
		named, problems := tsJobKindNames(opener + tc.body)
		if tc.want == nil {
			if len(problems) == 0 {
				t.Errorf("%s: parsed %v without a problem; this shape must fail loudly", name, named)
			}
			continue
		}
		if len(problems) != 0 || !slices.Equal(slices.Sorted(maps.Keys(named)), tc.want) {
			t.Errorf("%s: named %v, problems %q, want %v and none", name, named, problems, tc.want)
		}
	}
}
