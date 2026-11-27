// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A screen that reads a 404 as "nothing here yet" fills the browser console
// with failed requests on ordinary pages, and JavaScript cannot silence them.
// A read whose empty state is normal declares `Prefer: absent=204` and answers
// 204, and the client sends that preference on every read. A source that turns
// a 404 into null, undefined, an empty list or an absent state is one of two
// things. It is such a read not yet moved to 204, or a row-scope miss the
// screen keeps quiet about. The second kind says why in an `absent-404:`
// comment on one of the three lines above the comparison.

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const absent404Frontend = "../frontend/src"

// absent404Reason is the comment that keeps a 404 mapped to "none" with a reason.
const absent404Reason = "absent-404:"

// absent404Mapping matches a 404 comparison whose branch answers a none shape:
// on the same line, after a ternary, or as the first statement of its block.
var absent404Mapping = regexp.MustCompile(
	`===\s*404\b\)?\s*(?:\)\s*)?(?:\{\s*(?://[^\n]*\n\s*)*)?(?:return\s+|\?\s*)` +
		`(?:null\b|undefined\b|\[\]|\{\s*kind:\s*"absent")`)

// minimumFrontendSources is far under the tree's real count. A walk that reads
// fewer files has lost the tree, and would report a pass with nothing read.
const minimumFrontendSources = 500

func TestNoFrontendReadMapsA404ToNoneWithoutAReason(t *testing.T) {
	t.Parallel()
	read := 0
	var findings []string
	walkErr := gatekit.WalkDir(absent404Frontend, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isProductionFrontendSource(file) {
			return err
		}
		raw, readErr := gatekit.ReadFile(file)
		if readErr != nil {
			return readErr
		}
		read++
		rel, relErr := filepath.Rel(absent404Frontend, file)
		if relErr != nil {
			return relErr
		}
		findings = append(findings, absent404Findings(filepath.ToSlash(rel), string(raw))...)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the frontend: %v", walkErr)
	}
	if read < minimumFrontendSources {
		t.Fatalf("read %d frontend sources, expected at least %d: the walk lost the tree", read, minimumFrontendSources)
	}
	for _, finding := range findings {
		t.Error(finding)
	}
}

// isProductionFrontendSource is a shipped .ts/.tsx file. Tests, stories and
// fixtures stub a 404 to test it, and ship nothing.
func isProductionFrontendSource(path string) bool {
	if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
		return false
	}
	for _, marker := range []string{".test.", ".spec.", ".stories.", "testkit", "fixtures", ".testing.", "/test/", "/testing/"} {
		if strings.Contains(path, marker) {
			return false
		}
	}
	return true
}

// absent404Findings names each unexplained 404-to-none mapping in one source.
func absent404Findings(path, text string) []string {
	var found []string
	for _, match := range absent404Mapping.FindAllStringIndex(text, -1) {
		line := strings.Count(text[:match[0]], "\n")
		lines := strings.Split(text, "\n")
		if strings.Contains(strings.Join(lines[max(0, line-3):line+1], "\n"), absent404Reason) {
			continue
		}
		found = append(found, fmt.Sprintf("%s:%d reads a 404 as nothing there. If the read's empty state is "+
			"normal, declare Prefer: absent=204 on it in api/crm.yaml and read 204 instead; if this 404 is a row the "+
			"reader may not see, say so in an %q comment on one of the three lines above", path, line+1, absent404Reason))
	}
	return found
}

func TestTheAbsent404ScanSeesEveryShapeItGuards(t *testing.T) {
	t.Parallel()
	caught := map[string]string{
		"same line":       "if (response.status === 404) return null;",
		"block":           "if (response.status === 404) {\n  return null;\n}",
		"commented block": "if (response.status === 404) {\n  // nothing saved\n  return undefined;\n}",
		"empty list":      "if (status === 404) return [];",
		"absent state":    "if (response.status === 404)\n  return { kind: \"absent\", why: \"none\" };",
		"ternary":         "return problemStatus(error) === 404 ? null : error;",
		"helper call":     "if (problemStatus(error) === 404) return null;",
	}
	for name, source := range caught {
		if got := absent404Findings("probe.ts", source); len(got) != 1 {
			t.Errorf("%s: %d findings for %q, want 1", name, len(got), source)
		}
	}
	passed := map[string]string{
		"with a reason": "// absent-404: a hidden anchor and no draft are one answer.\nif (response.status === 404) return null;",
		"a refusal":     "if (response.status === 404) throw new Error(\"gone\");",
		"another code":  "if (response.status === 204) return null;",
		"a flag":        "return status === 404 || status === 410;",
	}
	for name, source := range passed {
		if got := absent404Findings("probe.ts", source); len(got) != 0 {
			t.Errorf("%s: %v, want no finding for %q", name, got, source)
		}
	}
}
