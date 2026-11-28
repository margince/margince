// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A lane runs on a change to every path it reads.
//
// A lane that reads a file outside its paths-filter scope can be broken by a
// pull request that never runs it. The red then lands on the next unrelated
// change that does trigger the lane.
//
// Many Go gates walk every tracked file, so their scope is `**`. The
// integration lane reads far less, so its census starts from the tests.

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	goGatesScope   = "backend"
	databaseScope  = "backend_db"
	minTrackedTree = 5000
)

func TestEveryTrackedFileTriggersTheGoGates(t *testing.T) {
	t.Parallel()
	scope := filterScopes(t, readWorkflow(t, classifierWorkflow))[goGatesScope]
	if len(scope) == 0 {
		t.Fatalf("%s declares no %q scope; the Go gates are guarded on it", classifierWorkflow, goGatesScope)
	}
	files := trackedFiles(t)
	if len(files) < minTrackedTree {
		t.Fatalf("the index lists %d files and the tree has at least %d; this census read the wrong repository",
			len(files), minTrackedTree)
	}
	var missing []string
	for _, file := range files {
		if !scopeClaims(file.path, scope) {
			missing = append(missing, file.path)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d tracked file(s) are outside ci.yml's %q scope, among them %s.\n"+
			"A pull request touching only those skips every Go gate, and many gates walk every tracked file. "+
			"Keep the scope `**`.", len(missing), goGatesScope, strings.Join(missing[:min(5, len(missing))], ", "))
	}
}

// candidatePath is a string literal that could name a file: no whitespace,
// and a separator or an extension.
var candidatePath = regexp.MustCompile(`"([A-Za-z0-9_.][A-Za-z0-9_./*-]*(?:/[A-Za-z0-9_./*-]*|\.[A-Za-z]{2,4}))"`)

// leadingJoinLiterals is the run of string literals a filepath.Join call starts
// with: `filepath.Join("..", "..", "config", "presets", preset)` reads
// ../../config/presets.
var leadingJoinLiterals = regexp.MustCompile(`filepath\.Join\(((?:\s*"[^"\\]+"\s*,?)+)`)

// rootJoinLiterals is a filepath.Join that starts from a helper naming the
// repository root: `filepath.Join(integration.RepositoryRoot(t), "docs", …)`.
var rootJoinLiterals = regexp.MustCompile(`filepath\.Join\(\s*(?:\w+\.)?(?:RepositoryRoot|repoRoot)\(t\)\s*,((?:\s*"[^"\\]+"\s*,?)+)`)

var quoted = regexp.MustCompile(`"([^"\\]+)"`)

func quotedParts(args string) []string {
	var parts []string
	for _, part := range quoted.FindAllStringSubmatch(args, -1) {
		parts = append(parts, part[1])
	}
	return parts
}

// integrationReads resolves the paths an integration-tagged file names relative
// to its own directory, the one `go test` runs it in, and keeps those outside
// backend/ that exist.
func integrationReads(file, body string) []string {
	var named []string
	for _, match := range candidatePath.FindAllStringSubmatch(body, -1) {
		named = append(named, match[1])
	}
	for _, match := range leadingJoinLiterals.FindAllStringSubmatch(body, -1) {
		named = append(named, path.Join(quotedParts(match[1])...))
	}
	// Spelled from the file's own directory, so it resolves as the others do.
	upToRoot := strings.Repeat("../", strings.Count(path.Dir(file), "/")+1)
	for _, match := range rootJoinLiterals.FindAllStringSubmatch(body, -1) {
		named = append(named, upToRoot+path.Join(quotedParts(match[1])...))
	}
	var reads []string
	for _, name := range named {
		if !strings.HasPrefix(name, "../") {
			continue
		}
		resolved := path.Clean(path.Join(path.Dir(file), name))
		if strings.HasPrefix(resolved, "../") || resolved == "backend" || strings.HasPrefix(resolved, "backend/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, strings.SplitN(resolved, "*", 2)[0])); err == nil {
			reads = append(reads, resolved)
		}
	}
	return reads
}

// trackedUnder is what a read of path reaches: the file itself, or every
// tracked file below it when it names a directory or a glob.
func trackedUnder(files []trackedFile, read string) []string {
	var reached []string
	for _, file := range files {
		if matched, err := path.Match(read, file.path); err == nil && matched {
			reached = append(reached, file.path)
		} else if file.path == read || strings.HasPrefix(file.path, read+"/") {
			reached = append(reached, file.path)
		}
	}
	return reached
}

func TestEveryPathAnIntegrationTestReadsTriggersTheDatabaseLanes(t *testing.T) {
	t.Parallel()
	scope := filterScopes(t, readWorkflow(t, classifierWorkflow))[databaseScope]
	files := trackedFiles(t)
	read := 0
	for _, file := range files {
		if !strings.HasPrefix(file.path, "backend/") || !strings.HasSuffix(file.path, ".go") {
			continue
		}
		onDisk := filepath.Join(repoRoot, file.path)
		onlyIntegration, err := builtOnlyForIntegration(onDisk)
		if err != nil {
			t.Fatalf("reading the build constraint of %s: %v", file.path, err)
		}
		if !onlyIntegration {
			continue
		}
		body, err := gatekit.ReadFile(onDisk)
		if err != nil {
			t.Fatalf("reading %s: %v", file.path, err)
		}
		for _, outside := range integrationReads(file.path, string(body)) {
			for _, reached := range trackedUnder(files, outside) {
				read++
				if !scopeClaims(reached, scope) {
					t.Errorf("%s reads %s, which ci.yml's %q scope does not claim. A pull request touching only "+
						"that file skips the integration lane. Add it to the scope.", file.path, reached, databaseScope)
				}
			}
		}
	}
	// Today: the compose stack, three root scripts, the role matrix and the
	// presets. A reader that stopped resolving would report none and pass.
	if read < 20 {
		t.Fatalf("this census found %d file(s) outside backend/ that integration tests read; the tree has at least 20", read)
	}
}

// The resolver reads both spellings a test uses, relative to its own directory.
// A path inside backend/ is left to the scope that already holds it.
func TestIntegrationReadsResolvesEachSpelling(t *testing.T) {
	t.Parallel()
	const file = "backend/internal/compose/x_integration_test.go"
	cases := []struct {
		body string
		want []string
	}{
		{`os.ReadFile(filepath.Join("..", "..", "..", "config", "presets", preset))`, []string{"config/presets"}},
		{`const dir = "../../../docs/reference/mcp-tool-roles"`, []string{"docs/reference/mcp-tool-roles"}},
		{`os.ReadFile("../../api/crm.yaml")`, nil},
		{`filepath.Join(dir, "config", "token")`, nil},
		{`filepath.Join(integration.RepositoryRoot(t), "docs", "reference", "mcp-tool-roles")`, []string{"docs/reference/mcp-tool-roles"}},
		{`filepath.Join(RepositoryRoot(t), "backend", "api")`, nil},
		{`"../../../no/such/path.txt"`, nil},
	}
	for _, c := range cases {
		got := integrationReads(file, c.body)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("integrationReads(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

// The matcher has to read each pattern the way paths-filter does. Otherwise a
// covered path is reported uncovered, and the fix is a duplicate filter line.
func TestScopeClaimsReadsEachFilterShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**", "LICENSE", true},
		{"**", ".claude/settings.json", true},
		{"docs/**", "docs/how-to/x.md", true},
		{"docs/**", "docs", true},
		{"docs/**", "docsx/a.md", false},
		{"Dockerfile*", "Dockerfile", true},
		{"Dockerfile*", "Dockerfile.dev", true},
		{"Dockerfile*", "docker/Dockerfile", false},
		{"README.md", "README.md", true},
		{"README.md", "README.mdx", false},
	}
	for _, c := range cases {
		if got := scopeClaims(c.path, []string{c.pattern}); got != c.want {
			t.Errorf("scopeClaims(%q, %q) = %v, want %v", c.path, c.pattern, got, c.want)
		}
	}
}
