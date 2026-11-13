// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bash32Breakers are tests a bash 3.2 parser rejects outright. macOS ships that
// bash, and a parse error there fails the whole script, not one line of it.
var bash32Breakers = regexp.MustCompile(`\[\[\s+(?:!\s+)?-v\s`)

// No shell script in the tree uses a test macOS bash cannot parse. Then make
// check runs on a contributor's Mac as it does on CI's Linux.
func TestNoShellScriptUsesATestBash32CannotParse(t *testing.T) {
	t.Parallel()
	read := 0
	err := gatekit.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || strings.HasPrefix(d.Name(), ".tmp")) {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		body, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		read++
		for _, line := range bash32BreakerLines(string(body)) {
			t.Errorf("%s:%d uses [[ -v ]], which macOS bash 3.2 cannot parse; test \"${VAR+set}\" instead", path, line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d shell scripts; the walk is short", read)
	}
}

// bash32BreakerLines names the lines where a [[ -v ]] test opens. Comment lines
// are blanked first, keeping the line count, so a test split over lines is
// matched as bash reads it.
func bash32BreakerLines(script string) []int {
	lines := strings.Split(script, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = ""
		}
	}
	code := strings.Join(lines, "\n")
	var found []int
	for _, at := range bash32Breakers.FindAllStringIndex(code, -1) {
		found = append(found, strings.Count(code[:at[0]], "\n")+1)
	}
	return found
}

// The matcher sees a test on one line and one split over lines, and leaves a
// comment that names the construct alone.
func TestTheBash32MatcherReadsATestAcrossLines(t *testing.T) {
	t.Parallel()
	for script, want := range map[string][]int{
		"if [[ ! -v FLAGS ]]; then\n  exit 2\nfi":          {1},
		"x=1\nif [[\n  ! -v FLAGS\n]]; then\n  exit 2\nfi": {2},
		"# never write [[ -v FLAGS ]]\nx=1":                nil,
		"if [[ -z \"${FLAGS+set}\" ]]; then exit; fi":      nil,
	} {
		if got := bash32BreakerLines(script); !slices.Equal(got, want) {
			t.Errorf("%q: lines %v, want %v", script, got, want)
		}
	}
}
