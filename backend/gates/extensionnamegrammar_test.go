// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every hand-typed copy of the `ext:<unit>:<short>` pattern carries the source
// pkg/extension publishes. The schema CHECKs, the contract and the frontend
// each restate it. A copy that drifts refuses a name the Go validators accept,
// or admits one they refuse. Copies are found by their `ext:` text, so a new
// one is read without being listed here.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/pkg/extension"
)

// qualifiedCopySource is one file that restates the pattern, and the fewest
// copies it held when this gate was written.
type qualifiedCopySource struct {
	path  string
	floor int
	// copyLine picks the lines that state a pattern rather than mention one.
	copyLine func(line string) bool
}

// companyTypeContext is the word near a copy that marks it as a company type,
// the one family that keeps its own short rule.
const companyTypeContext = "relationship_type"

func TestEveryCopyOfTheExtensionNamePatternIsThePublishedOne(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	for _, source := range []qualifiedCopySource{
		{
			path: filepath.Join(root, "migrations", "testdata", "head_catalog.txt"), floor: 10,
			copyLine: func(line string) bool { return strings.Contains(line, extension.QualifiedPrefix) },
		},
		{
			path: filepath.Join(root, "api", "crm.yaml"), floor: 11,
			copyLine: func(line string) bool { return strings.Contains(line, "pattern:") },
		},
		{
			path: filepath.Join(root, "..", "frontend", "src", "lib", "entity.ts"), floor: 1,
			copyLine: func(line string) bool { return strings.Contains(line, "EXTENSION_KIND =") },
		},
	} {
		t.Run(filepath.Base(source.path), func(t *testing.T) {
			t.Parallel()
			body, err := gatekit.ReadFile(source.path)
			if err != nil {
				t.Fatalf("reading %s: %v", source.path, err)
			}
			lines := strings.Split(string(body), "\n")
			seen := 0
			for i, line := range lines {
				if !source.copyLine(line) {
					continue
				}
				problems, copies := qualifiedCopyProblems(line, nearCompanyType(lines, i))
				for _, problem := range problems {
					t.Errorf("%s:%d %s\n\t%s", filepath.Base(source.path), i+1, problem, line)
				}
				seen += copies
			}
			if seen < source.floor {
				t.Fatalf("read %d copies of the pattern in %s, want at least the %d this gate was written against: the file or the way a copy is spelled changed shape",
					seen, source.path, source.floor)
			}
		})
	}
}

// qualifiedCopyProblems checks each `ext:` in a line and counts the ones that
// state a whole pattern. A bare prefix test (`~ '^ext:'`, `LIKE 'ext:%'`)
// states none and passes; any other must open with one of the two published
// bodies and end right after it.
func qualifiedCopyProblems(line string, companyType bool) (problems []string, copies int) {
	kind, company := patternBody(extension.QualifiedNamePattern), patternBody(extension.CompanyTypeNamePattern)
	for rest := line; ; {
		at := strings.Index(rest, extension.QualifiedPrefix)
		if at < 0 {
			return problems, copies
		}
		rest = rest[at:]
		if strings.HasPrefix(rest, extension.QualifiedPrefix+"'") || strings.HasPrefix(rest, extension.QualifiedPrefix+"%") {
			rest = rest[len(extension.QualifiedPrefix):]
			continue
		}
		copies++
		switch {
		case closesAfter(rest, kind):
		case closesAfter(rest, company) && companyType:
		case closesAfter(rest, company):
			problems = append(problems, "carries the company-type pattern on a column that is not a company type: use extension.QualifiedNamePattern")
		default:
			problems = append(problems, "states an `ext:` pattern that is neither extension.QualifiedNamePattern nor extension.CompanyTypeNamePattern: copy the published source")
		}
		rest = rest[len(extension.QualifiedPrefix):]
	}
}

// closesAfter reports that s opens with body and the pattern ends there, so a
// copy with a longer tail (`{0,31}x`) is not read as a match.
func closesAfter(s, body string) bool {
	tail, ok := strings.CutPrefix(s, body)
	return ok && (strings.HasPrefix(tail, "$") || strings.HasPrefix(tail, ")$"))
}

func patternBody(pattern string) string {
	return strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
}

// nearCompanyType reports whether the copy at lines[i] belongs to a company
// type. The catalog names the column on the line. The contract names the
// property a few lines above its pattern.
func nearCompanyType(lines []string, i int) bool {
	for j := max(0, i-6); j <= i; j++ {
		if strings.Contains(lines[j], companyTypeContext) {
			return true
		}
	}
	return false
}

func TestTheNamePatternGateReadsADriftedCopy(t *testing.T) {
	t.Parallel()
	good := `pattern: '^(contact|company)$|` + extension.QualifiedNamePattern + `'`
	if problems, _ := qualifiedCopyProblems(good, false); len(problems) != 0 {
		t.Fatalf("the published pattern was refused: %v", problems)
	}
	for name, line := range map[string]string{
		"shorter cap":        strings.Replace(good, "{0,31}", "{0,30}", 1),
		"longer tail":        strings.Replace(good, "{0,31}$", "{0,31}x$", 1),
		"company rule":       `pattern: '` + extension.CompanyTypeNamePattern + `'`,
		"second copy drifts": good + ` or '^ext:[a-z]+:[a-z]+$'`,
	} {
		if problems, _ := qualifiedCopyProblems(line, false); len(problems) == 0 {
			t.Errorf("%s: a drifted copy passed\n\t%s", name, line)
		}
	}
	if problems, _ := qualifiedCopyProblems(`pattern: '`+extension.CompanyTypeNamePattern+`'`, true); len(problems) != 0 {
		t.Errorf("the company-type pattern was refused on a company type: %v", problems)
	}
	if problems, _ := qualifiedCopyProblems(`WHEN (entity_type ~ '^ext:'::text) THEN 0`, false); len(problems) != 0 {
		t.Errorf("a bare prefix test was read as a pattern: %v", problems)
	}
}
