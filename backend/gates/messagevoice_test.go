// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Who wrote a message has one answer: Activity.voice. A model reads it through
// contactbrief.SpeakerFor and a screen through frontend/src/lib/messagevoice.ts.
// This cannot see a screen that guesses an author and prints no author key.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	speakerOwnerFile  = "internal/compose/contactbrief/speaker.go"
	speakerOwnerFunc  = "SpeakerFor"
	speakerType       = "Speaker"
	voiceHelperImport = "lib/messagevoice"
	voiceFrontendRoot = "../frontend/src"
	voiceKeyCatalog   = "../frontend/src/i18n/en.ts"
)

// TestOnlySpeakerForBuildsABriefSpeaker fails on a Speaker literal outside
// SpeakerFor: the only other way to hand a model a speaker is a second mapping.
func TestOnlySpeakerForBuildsABriefSpeaker(t *testing.T) {
	t.Parallel()
	owned := 0
	var offenders []string
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			inOwner := ok && fn.Name.Name == speakerOwnerFunc && rel == speakerOwnerFile
			ast.Inspect(decl, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok || !namesSpeakerType(lit.Type, rel) {
					return true
				}
				if inOwner {
					owned++
				} else {
					offenders = append(offenders, rel)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}
	if owned == 0 {
		t.Fatalf("found no Speaker literal in %s.%s; the census reads nothing and would pass vacuously",
			speakerOwnerFile, speakerOwnerFunc)
	}
	for _, offender := range offenders {
		t.Errorf("%s builds a brief Speaker itself. Fold the row's voice through contactbrief.SpeakerFor, "+
			"so every brief names an author by one rule", offender)
	}
}

// namesSpeakerType reports a literal of contactbrief's Speaker: unqualified
// inside that package, `contactbrief.Speaker` outside it.
func namesSpeakerType(expr ast.Expr, rel string) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == speakerType && strings.HasPrefix(rel, "internal/compose/contactbrief/")
	case *ast.SelectorExpr:
		pkg, ok := typed.X.(*ast.Ident)
		return ok && pkg.Name == "contactbrief" && typed.Sel.Name == speakerType
	}
	return false
}

// authorSideKey matches a catalog key whose English text says which side wrote.
var authorSideKey = regexp.MustCompile(`"([a-z][a-zA-Z0-9.]*\.(?:theyWrote|youWrote|weWrote|wroteBy))":`)

// TestAScreenThatNamesAnAuthorReadsTheProjection fails on a frontend file that
// prints an author-side word beside a `.direction` read without importing the
// shared helper.
func TestAScreenThatNamesAnAuthorReadsTheProjection(t *testing.T) {
	t.Parallel()
	catalog, err := os.ReadFile(voiceKeyCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", voiceKeyCatalog, err)
	}
	var keys []string
	for _, m := range authorSideKey.FindAllStringSubmatch(string(catalog), -1) {
		keys = append(keys, m[1])
	}
	if len(keys) == 0 {
		t.Fatalf("found no author-side key in %s; the census reads nothing", voiceKeyCatalog)
	}
	readsDirection := regexp.MustCompile(`\.direction\b`)
	checked := 0
	err = filepath.WalkDir(voiceFrontendRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") ||
			strings.Contains(path, ".test.") || strings.Contains(path, ".stories.") ||
			strings.Contains(filepath.ToSlash(path), "/i18n/") || strings.HasSuffix(path, "schema.d.ts") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		code := string(body)
		if !usesAnyKey(code, keys) || !readsDirection.MatchString(code) {
			return nil
		}
		checked++
		if !strings.Contains(code, voiceHelperImport) {
			t.Errorf("%s prints who wrote a message beside a direction it reads itself. Name the author "+
				"through %s, which reads the server's voice projection", filepath.ToSlash(path), voiceHelperImport)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", voiceFrontendRoot, err)
	}
	if checked == 0 {
		t.Fatal("no frontend file prints an author-side word beside a direction; the census reads nothing " +
			"and would pass vacuously")
	}
}

func usesAnyKey(code string, keys []string) bool {
	for _, key := range keys {
		if strings.Contains(code, `"`+key+`"`) {
			return true
		}
	}
	return false
}
