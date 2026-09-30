// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A read in notices that asks whether a line is unread also asks whether
// somebody else's act took it back.
//
// A notice saying a decision waits on the reader stops being true the moment a
// colleague decides it, and the reader did nothing. Four surfaces asked only
// the first half — the Worklist lane, the centre's badge, the morning's window
// and the mail claim — and the count the centre answers a clearing with asked
// it a fifth time. They were found by hand once. A sixth reader either composes
// the module's own standing fragment or fails here.
//
// STATEMENTS, NOT LINES. The SQL is a Go string that wraps and is assembled
// from fragments, so a line-scanning scan judges half a predicate and a census
// that can fail short has already failed: it reads a smaller module, reports
// PASS, and leaves no assertion to notice. The walk parses each file and judges
// the DECLARATION holding the literal, with the fragments resolved by name from
// the module itself rather than listed here.
//
// THE EXEMPTION IS DERIVED, not a list: a statement that settles the reader's
// own act and reports nothing back. MarkRead claims the row by `read_at IS
// NULL` to make a repeated settle a no-op, and an overtaken line is still the
// reader's to open — narrowing that claim would refuse the line the centre
// deliberately still lists. A statement that REPORTS, through RETURNING or a
// count, is telling the reader a number and asks both.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// unreadArm is the question every surface used to ask alone, and
	// overtakenArm the one that now stands beside it. Matched as text because
	// that is what Go hands the database.
	unreadArm    = "read_at IS NULL"
	overtakenArm = "overtaken_at IS NULL"

	// settleClaim and reportsBack tell a settle from a report. A statement
	// carrying the first and not the second moves the reader's own rows and
	// answers with nothing, so it has no figure to be wrong about.
	settleClaim = "SET read_at = now()"
	reportsBack = "RETURNING"

	// noticeSurfaces is how many places tell a reader something is waiting.
	// Under-recognition is the one way this gate must not break, so the floor
	// is asserted: a walk that stopped seeing the module would otherwise report
	// the same PASS as a module where every read asks both.
	noticeSurfaces = 5
)

// noticesModule is the subject. The table's writers and readers are all here —
// the module owns `notice` and nothing else writes it — so the module IS the
// corpus rather than a sample of it.
var noticesModule = filepath.Join(modulesDir, "notices")

// noticesFile is one parsed source file of the module, kept with its path so a
// finding names the file the author has to open.
type noticesFile struct {
	path string
	file *ast.File
}

func TestEveryUnreadReadInNoticesAlsoAsksWhetherItWasOvertaken(t *testing.T) {
	t.Parallel()

	files := parseNoticesModule(t)
	carrying := fragmentsCarryingTheArm(files)
	if len(carrying) == 0 {
		t.Fatalf("no constant in %s names %q, so there is no fragment for a reader to compose "+
			"and nothing here judges anything", noticesModule, overtakenArm)
	}

	asking, composing := 0, 0
	for _, source := range files {
		for _, decl := range source.file.Decls {
			if namesTheArm(decl, carrying) {
				composing++
			}
			statements, mustAsk := unreadStatements(decl)
			if len(statements) == 0 {
				continue
			}
			asking++
			if !mustAsk || namesTheArm(decl, carrying) {
				continue
			}
			t.Errorf("%s: %s asks whether a line is unread and not whether it was overtaken:\n\t%s\n"+
				"\tA line a colleague's decision took back still claims a decision waits on this "+
				"reader, and they can do nothing about it.\n"+
				"\tCompose the module's standing fragment instead of spelling the filter again.",
				source.path, declName(decl), gatekit.FirstLineOf(statements[0]))
		}
	}

	if asking == 0 {
		t.Fatalf("no declaration in %s names %q — the walk is broken, and a census of nothing "+
			"reports the clean module it never read", noticesModule, unreadArm)
	}
	if composing < noticeSurfaces {
		t.Fatalf("%d declaration(s) in %s compose the standing fragment, and the lane, the badge, "+
			"the morning, the mail claim and the clearing count are %d — either the walk stopped "+
			"resolving the fragment, or a surface stopped asking",
			composing, noticesModule, noticeSurfaces)
	}
}

// The reader is asserted against planted source, because the failure this gate
// must never have is under-recognition: a walk that stopped matching reports
// the same green as a module where every read asks both. Both directions, since
// a reader that accepted everything would pass the module it was written for.
func TestTheUnreadReaderSeesAFilterSpelledAgain(t *testing.T) {
	t.Parallel()

	planted := parseSource(t, `package notices

const standing = "read_at IS NULL AND overtaken_at IS NULL"

func composes() string { return "WHERE recipient_user_id = $1 AND " + standing }

func spellsItsOwn() string { return "WHERE recipient_user_id = $1 AND read_at IS NULL" }

func settles() string { return "UPDATE notice SET read_at = now() WHERE id = $1 AND read_at IS NULL" }
`)
	carrying := fragmentsCarryingTheArm([]noticesFile{planted})
	if !carrying["standing"] {
		t.Fatal("the fragment naming the overtaken arm was not recognised, so every composing reader would read as a finding")
	}

	for _, want := range []struct {
		decl    string
		asks    bool
		mustAsk bool
		arm     bool
	}{
		{decl: "composes", asks: false, arm: true},
		{decl: "spellsItsOwn", asks: true, mustAsk: true, arm: false},
		{decl: "settles", asks: true, mustAsk: false, arm: false},
	} {
		decl := declNamed(t, planted.file, want.decl)
		statements, mustAsk := unreadStatements(decl)
		if asks := len(statements) > 0; asks != want.asks {
			t.Errorf("%s: names the unread filter itself = %v, want %v", want.decl, asks, want.asks)
		}
		if mustAsk != want.mustAsk {
			t.Errorf("%s: has a figure to be wrong about = %v, want %v", want.decl, mustAsk, want.mustAsk)
		}
		if arm := namesTheArm(decl, carrying); arm != want.arm {
			t.Errorf("%s: asks the overtaken question = %v, want %v", want.decl, arm, want.arm)
		}
	}
}

// parseNoticesModule parses the module's own source, in path order so a run
// reports its findings the same way twice. Tests are excluded: a case seeding a
// row by hand is not a surface anybody is shown.
func parseNoticesModule(t *testing.T) []noticesFile {
	t.Helper()
	entries, err := os.ReadDir(noticesModule)
	if err != nil {
		t.Fatalf("reading %s: %v", noticesModule, err)
	}
	var files []noticesFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(noticesModule, name)
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, noticesFile{path: path, file: file})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files
}

// fragmentsCarryingTheArm names the module's constants that ask the overtaken
// question, directly or through another one. To a fixed point, because a
// fragment built from a fragment is how this module already composes its SQL
// and a single pass would resolve them in file order.
func fragmentsCarryingTheArm(files []noticesFile) map[string]bool {
	carrying := map[string]bool{}
	for grew := true; grew; {
		grew = false
		for _, source := range files {
			for _, decl := range source.file.Decls {
				gen, isGen := decl.(*ast.GenDecl)
				if !isGen || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, isValue := spec.(*ast.ValueSpec)
					if !isValue {
						continue
					}
					for i, name := range value.Names {
						if carrying[name.Name] || i >= len(value.Values) {
							continue
						}
						if namesTheArm(value.Values[i], carrying) {
							carrying[name.Name] = true
							grew = true
						}
					}
				}
			}
		}
	}
	return carrying
}

// namesTheArm reports whether the subtree asks the overtaken question — in its
// own text, or by naming a fragment that does.
func namesTheArm(node ast.Node, carrying map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.BasicLit:
			if typed.Kind == token.STRING && strings.Contains(gatekit.TextOf(typed), overtakenArm) {
				found = true
			}
		case *ast.Ident:
			if carrying[typed.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// unreadStatements returns the declaration's own spellings of the unread
// filter, and whether any of them reports a figure back to the reader.
func unreadStatements(decl ast.Decl) (statements []string, mustAsk bool) {
	ast.Inspect(decl, func(n ast.Node) bool {
		lit, isLit := n.(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		sql := gatekit.TextOf(lit)
		if !strings.Contains(sql, unreadArm) {
			return true
		}
		statements = append(statements, sql)
		if !strings.Contains(sql, settleClaim) || strings.Contains(sql, reportsBack) {
			mustAsk = true
		}
		return true
	})
	return statements, mustAsk
}

// declName is what a finding calls the declaration: the function, or the first
// constant a shared fragment declares.
func declName(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return typed.Name.Name
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			if value, isValue := spec.(*ast.ValueSpec); isValue && len(value.Names) > 0 {
				return value.Names[0].Name
			}
		}
	}
	return "a declaration"
}

func parseSource(t *testing.T, src string) noticesFile {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	return noticesFile{path: "planted.go", file: file}
}

func declNamed(t *testing.T, file *ast.File, name string) ast.Decl {
	t.Helper()
	for _, decl := range file.Decls {
		if declName(decl) == name {
			return decl
		}
	}
	t.Fatalf("the planted source declares no %s", name)
	return nil
}
