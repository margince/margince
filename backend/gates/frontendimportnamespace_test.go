// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser reads a row as imported when its `source` starts with the
// importer's reserved namespace. Spelled differently on the two sides, every
// imported row reads "typed by you" to the administrator who ran the import.
//
// So IMPORT_NAMESPACE is a declared mirror of provenance.ReservedSourceSystemPrefix.
// A change on either side fails, and so does a second hand-typed copy.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const (
	frontendSrc             = "../frontend/src"
	frontendImportNamespace = "../frontend/src/design-system/provenanceof.ts"
)

// Matched by name, in any quote a string literal admits, so a re-quoted
// declaration is still read instead of counted as gone.
var tsImportNamespace = regexp.MustCompile("const IMPORT_NAMESPACE\\s*=\\s*[\"'`]([^\"'`]*)[\"'`]")

// importNamespaceDeclarations reads the mirrored value out of TypeScript text.
// Comments go first, so a line that only mentions the declaration is not one.
func importNamespaceDeclarations(source string) []string {
	var values []string
	for _, m := range tsImportNamespace.FindAllStringSubmatch(tsComment.ReplaceAllString(source, " "), -1) {
		values = append(values, m[1])
	}
	return values
}

// namespaceLiterals counts string literals equal to the prefix. Each is a copy
// of the backend constant, and only the declared one is checked.
func namespaceLiterals(source, prefix string) int {
	literal := regexp.MustCompile("[\"'`]" + regexp.QuoteMeta(prefix) + "[\"'`]")
	return len(literal.FindAllStringIndex(tsComment.ReplaceAllString(source, " "), -1))
}

// isProductTypeScript keeps tests out of the census: a test may name the
// prefix to assert it is never shown, and decides nothing a reader sees.
func isProductTypeScript(path string) bool {
	return (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) && !strings.Contains(path, ".test.")
}

func TestTheFrontendImportNamespaceMirrorsTheBackendPrefix(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendImportNamespace)
	if err != nil {
		t.Fatalf("read %s: %v", frontendImportNamespace, err)
	}
	declared := importNamespaceDeclarations(string(source))
	if len(declared) != 1 {
		t.Fatalf("%s declares IMPORT_NAMESPACE %d times, want once: the browser no longer has one mirror of provenance.ReservedSourceSystemPrefix", frontendImportNamespace, len(declared))
	}
	if declared[0] != provenance.ReservedSourceSystemPrefix {
		t.Fatalf("import namespace disagrees: server %q, %s %q — every imported row would read as typed by the administrator who ran the import",
			provenance.ReservedSourceSystemPrefix, frontendImportNamespace, declared[0])
	}

	copies := map[string]int{}
	walked := 0
	walkErr := filepath.WalkDir(frontendSrc, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isProductTypeScript(path) {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		walked++
		if n := namespaceLiterals(string(body), provenance.ReservedSourceSystemPrefix); n > 0 {
			copies[filepath.ToSlash(path)] = n
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", frontendSrc, walkErr)
	}
	if walked == 0 {
		t.Fatalf("no TypeScript read under %s — a census of nothing agrees with everything", frontendSrc)
	}
	if copies[frontendImportNamespace] != 1 || len(copies) != 1 {
		t.Errorf("the literal %q must appear once in the frontend/src product code, as IMPORT_NAMESPACE in %s; found %v. Import the constant instead of typing the prefix again",
			provenance.ReservedSourceSystemPrefix, frontendImportNamespace, copies)
	}
}

func TestTheImportNamespaceCensusSeesEveryShapeOfACopy(t *testing.T) {
	t.Parallel()
	prefix := provenance.ReservedSourceSystemPrefix
	cases := []struct {
		name     string
		source   string
		declared int
		literals int
	}{
		{"double-quoted declaration", `const IMPORT_NAMESPACE = "` + prefix + `";`, 1, 1},
		{"single-quoted declaration", `const IMPORT_NAMESPACE = '` + prefix + `';`, 1, 1},
		{"declaration only in a comment", "// const IMPORT_NAMESPACE = \"" + prefix + "\";\n/* \"" + prefix + "\" */", 0, 0},
		{"a second copy beside the declaration", `const IMPORT_NAMESPACE = "` + prefix + `"; s.startsWith("` + prefix + `")`, 1, 2},
		{"a full stamp is data, not a copy", `const row = { source: "` + prefix + `hubspot" };`, 0, 0},
	}
	for _, c := range cases {
		if got := len(importNamespaceDeclarations(c.source)); got != c.declared {
			t.Errorf("%s: read %d declarations, want %d", c.name, got, c.declared)
		}
		if got := namespaceLiterals(c.source, prefix); got != c.literals {
			t.Errorf("%s: counted %d literals, want %d", c.name, got, c.literals)
		}
	}
	for path, want := range map[string]bool{"a/b.ts": true, "a/b.tsx": true, "a/b.stories.tsx": true, "a/b.test.ts": false, "a/b.md": false} {
		if got := isProductTypeScript(path); got != want {
			t.Errorf("isProductTypeScript(%q) = %v, want %v", path, got, want)
		}
	}
}
