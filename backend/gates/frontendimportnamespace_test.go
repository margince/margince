// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser parses the CSV importer's source stamp to say a record was
// imported, and from which upload.
//
// The three parts of that grammar it reads are declared mirrors of the server's:
// IMPORT_NAMESPACE of provenance.ReservedSourceSystemPrefix, IMPORT_CONNECTOR
// of migration.ConnectorCSV, IMPORT_OBJECTS of migration.ImportOrder. A drift
// in either direction fails, and so does a second hand-typed namespace.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const (
	frontendSrc           = "../frontend/src"
	frontendImportGrammar = "../frontend/src/design-system/provenanceof.ts"
)

// tsStringConst reads `const <name> = "<value>"` in any quote a string literal
// admits, so a re-quoted declaration is still read instead of counted as gone.
func tsStringConst(source, name string) []string {
	decl := regexp.MustCompile(`const ` + name + "\\s*=\\s*[\"'`]([^\"'`]*)[\"'`]")
	var values []string
	for _, m := range decl.FindAllStringSubmatch(tsComment.ReplaceAllString(source, " "), -1) {
		values = append(values, m[1])
	}
	return values
}

var (
	tsImportObjects = regexp.MustCompile(`const IMPORT_OBJECTS\s*=\s*new Set\(\[([^\]]*)\]\)`)
	tsQuoted        = regexp.MustCompile("[\"'`]([^\"'`]*)[\"'`]")
)

// tsImportObjectSet reads the Set's members. Anything in the brackets that is
// not a quoted word (a spread, a constant) is refused, because a member this
// reader cannot see is one the comparison would silently agree with.
func tsImportObjectSet(source string) ([]string, error) {
	matches := tsImportObjects.FindAllStringSubmatch(tsComment.ReplaceAllString(source, " "), -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("IMPORT_OBJECTS declared %d times as new Set([...]), want once", len(matches))
	}
	body := matches[0][1]
	var members []string
	for _, m := range tsQuoted.FindAllStringSubmatch(body, -1) {
		members = append(members, m[1])
	}
	if residue := strings.Trim(tsQuoted.ReplaceAllString(body, ""), " ,\n\t"); residue != "" {
		return nil, fmt.Errorf("IMPORT_OBJECTS holds %q, which is not a quoted object name", residue)
	}
	return members, nil
}

// namespaceCopies counts literals that open with the namespace word: a string,
// a template, a regexp source or a regexp literal, anchored or not. Each is a
// predicate on the stamp the declared mirror no longer governs.
func namespaceCopies(source, prefix string) int {
	word := regexp.QuoteMeta(strings.TrimSuffix(prefix, ":"))
	copyOf := regexp.MustCompile("[\"'`/]\\^?" + word + "(?:" + regexp.QuoteMeta(":") + "|[\"'`/])")
	return len(copyOf.FindAllStringIndex(tsComment.ReplaceAllString(source, " "), -1))
}

// isProductTypeScript keeps tests and stories out of the census. Both may hold
// a stamp as data (leadreadings.stories.tsx renders a `source_system` value)
// and neither decides what a reader of the product sees.
func isProductTypeScript(path string) bool {
	return (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) &&
		!strings.Contains(path, ".test.") && !strings.Contains(path, ".stories.")
}

func TestTheFrontendImportGrammarMirrorsTheImporter(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(frontendImportGrammar)
	if err != nil {
		t.Fatalf("read %s: %v", frontendImportGrammar, err)
	}
	source := string(raw)
	for _, mirror := range []struct{ name, server, cost string }{
		{"IMPORT_NAMESPACE", provenance.ReservedSourceSystemPrefix, "an imported record would read as typed by the administrator who ran the import"},
		{"IMPORT_CONNECTOR", migration.ConnectorCSV, "every imported record would lose its upload's name"},
	} {
		got := tsStringConst(source, mirror.name)
		if len(got) != 1 {
			t.Errorf("%s declares %s %d times, want once", frontendImportGrammar, mirror.name, len(got))
			continue
		}
		if got[0] != mirror.server {
			t.Errorf("%s disagrees: server %q, browser %q — %s", mirror.name, mirror.server, got[0], mirror.cost)
		}
	}
	objects, err := tsImportObjectSet(source)
	if err != nil {
		t.Fatalf("%s: %v", frontendImportGrammar, err)
	}
	for _, object := range migration.ImportOrder {
		if !slices.Contains(objects, object) {
			t.Errorf("IMPORT_OBJECTS lacks %q from migration.ImportOrder: a record of it would lose its upload's name", object)
		}
	}
	for _, object := range objects {
		if !slices.Contains(migration.ImportOrder, object) {
			t.Errorf("IMPORT_OBJECTS holds %q, which migration.ImportOrder does not import: the browser reads a stamp no importer writes", object)
		}
	}
}

func TestTheImportNamespaceIsTypedOnceInTheFrontend(t *testing.T) {
	t.Parallel()
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
		if n := namespaceCopies(string(body), provenance.ReservedSourceSystemPrefix); n > 0 {
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
	if copies[frontendImportGrammar] != 1 || len(copies) != 1 {
		t.Errorf("the namespace %q must be typed once in frontend/src product code, as IMPORT_NAMESPACE in %s; found %v. Call importProvenance instead of testing the stamp again",
			provenance.ReservedSourceSystemPrefix, frontendImportGrammar, copies)
	}
}

func TestTheImportGrammarReaderSeesQuotedDeclarationsAndPredicateCopies(t *testing.T) {
	t.Parallel()
	prefix := provenance.ReservedSourceSystemPrefix
	cases := []struct {
		name     string
		source   string
		declared int
		copies   int
	}{
		{"double-quoted declaration", `const IMPORT_NAMESPACE = "` + prefix + `";`, 1, 1},
		{"single-quoted declaration", `const IMPORT_NAMESPACE = '` + prefix + `';`, 1, 1},
		{"backtick declaration", "const IMPORT_NAMESPACE = `" + prefix + "`;", 1, 1},
		{"declaration only in a comment", "// const IMPORT_NAMESPACE = \"" + prefix + "\";\n/* \"" + prefix + "\" */", 0, 0},
		{"startsWith on a longer stamp", `s.startsWith("` + prefix + `csv:")`, 0, 1},
		{"anchored regexp literal", `/^` + prefix + `/.test(s)`, 0, 1},
		{"regexp built from a string", `new RegExp("^` + prefix + `csv")`, 0, 1},
		{"template literal", "s === `" + prefix + "csv:${name}`", 0, 1},
		{"the word without its colon", `s.split(":")[0] === "` + strings.TrimSuffix(prefix, ":") + `"`, 0, 1},
		{"a longer word is not the namespace", `const label = "` + strings.TrimSuffix(prefix, ":") + `ed";`, 0, 0},
	}
	for _, c := range cases {
		if got := len(tsStringConst(c.source, "IMPORT_NAMESPACE")); got != c.declared {
			t.Errorf("%s: read %d declarations, want %d", c.name, got, c.declared)
		}
		if got := namespaceCopies(c.source, prefix); got != c.copies {
			t.Errorf("%s: counted %d copies, want %d", c.name, got, c.copies)
		}
	}
	if got, err := tsImportObjectSet(`const IMPORT_OBJECTS = new Set(["company", 'lead']);`); err != nil || !slices.Equal(got, []string{"company", "lead"}) {
		t.Errorf("quoted members: read %v, %v", got, err)
	}
	if _, err := tsImportObjectSet(`const IMPORT_OBJECTS = new Set(["company", ...MORE]);`); err == nil {
		t.Error("a spread member was read as nothing instead of refused")
	}
	for path, want := range map[string]bool{"a/b.ts": true, "a/b.tsx": true, "a/b.stories.tsx": false, "a/b.test.ts": false, "a/b.md": false} {
		if got := isProductTypeScript(path); got != want {
			t.Errorf("isProductTypeScript(%q) = %v, want %v", path, got, want)
		}
	}
}
