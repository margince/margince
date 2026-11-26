// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The German the server writes uses no word the style page retires. The
// Never column of docs/reference/ui-copy-style-de.md owns the list, and the
// frontend census reads the same column, so neither side keeps a copy.
//
// German is found by the shape that marks it, so a new catalog is read the day
// it is written. The shapes:
//
//   - the value of a key named De, German or german, or textlang.German;
//   - the body of a switch case on German or textlang.German;
//   - the middle of an English, German, Vietnamese row of string literals.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// germanCopyRoots are the Go trees that ship with the product: this module and
// the extension units, which are separate modules.
var germanCopyRoots = []string{".", "../extensions"}

const germanStylePage = "../docs/reference/ui-copy-style-de.md"

// The floors sit below today's counts, so a walk that stopped reaching the tree
// fails instead of passing over nothing.
const (
	germanStringFloor   = 2500
	germanRowFloor      = 200
	germanCaseFloor     = 5
	retiredWordFloor    = 100
	germanLetters       = `A-Za-zÄÖÜäöüß`
	vietnameseOnlyRunes = "ăâđêôơưĂÂĐÊÔƠƯạảấầẩẫậắằẳẵặẹẻẽếềểễệỉịọỏốồổỗộớờởỡợụủứừửữựỳỵỷỹ"
)

// germanString is one German string literal and the shape that marked it.
type germanString struct {
	at    string
	shape string
	text  string
}

func TestServerGermanUsesNoRetiredWord(t *testing.T) {
	t.Parallel()
	retired := retiredGermanWords(t)
	found := serverGermanStrings(t)
	shapes := map[string]int{}
	for _, one := range found {
		shapes[one.shape]++
	}
	if len(found) < germanStringFloor || shapes[shapeRow] < germanRowFloor || shapes[shapeCase] < germanCaseFloor {
		t.Fatalf("found %d German strings (%v); the tree holds more, so this walk reads a smaller tree "+
			"than the one it judges", len(found), shapes)
	}
	for _, finding := range retiredWordFindings(found, retired) {
		t.Error(finding)
	}
}

// TestTheGermanVocabularyGateSeesEveryShape plants a retired word in each shape
// the walk reads, so a walk that stops seeing one fails here.
func TestTheGermanVocabularyGateSeesEveryShape(t *testing.T) {
	t.Parallel()
	planted := `package planted

var phrase = langcopy.Phrase{En: "Company", De: "Firma", Vi: "Công ty"}
var byLanguage = map[textlang.Lang]copy{textlang.German: {Title: "Neue Firma"}}
var mail = map[Language]Copy{German: {Subject: "Firma"}}
var row = vocabularyEntry{"Company", "Firma", "Công ty"}

func label(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return "Firma"
	}
	return ""
}

func lines(line writeLine) {
	line(field, "Company", "Firma", "Công ty")
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", planted, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the planted shapes: %v", err)
	}
	found := germanStringsIn(token.NewFileSet(), "planted.go", file)
	findings := retiredWordFindings(found, []string{"Firma"})
	if len(findings) != 6 {
		t.Errorf("the walk finds %d of the 6 planted retired words: %v", len(findings), findings)
	}
	compound := retiredWordFindings([]germanString{{text: "Forecast-Kategorie, Nutzerkonto"}}, []string{"Forecast", "Nutzer"})
	if len(compound) != 1 {
		t.Errorf("a whole word in a hyphenated compound is found and a closed compound is not: %v", compound)
	}
}

// retiredGermanWords reads the code-formatted words of the Never column.
func retiredGermanWords(t *testing.T) []string {
	t.Helper()
	page, err := gatekit.ReadFile(germanStylePage)
	if err != nil {
		t.Fatalf("reading the German style page that owns the retired words: %v", err)
	}
	_, section, ok := strings.Cut(string(page), "\n## Vocabulary\n")
	if !ok {
		t.Fatalf("%s has no Vocabulary section, which owns the retired words", germanStylePage)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var words []string
	rows := 0
	for line := range strings.Lines(section) {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		rows++
		cells := strings.Split(line, "|")
		if rows == 1 || len(cells) < 4 {
			continue
		}
		for _, match := range neverWord.FindAllStringSubmatch(cells[3], -1) {
			words = append(words, match[1])
		}
	}
	if len(words) < retiredWordFloor {
		t.Fatalf("read %d retired words from the Never column; it holds more, so the parse broke", len(words))
	}
	return words
}

var neverWord = regexp.MustCompile("`([^`]+)`")

func retiredWordFindings(found []germanString, retired []string) []string {
	patterns := make([]*regexp.Regexp, len(retired))
	for i, word := range retired {
		patterns[i] = regexp.MustCompile(`(?:^|[^` + germanLetters + `])` + regexp.QuoteMeta(word) + `(?:$|[^` + germanLetters + `])`)
	}
	var findings []string
	for _, one := range found {
		for i, pattern := range patterns {
			if pattern.MatchString(one.text) {
				findings = append(findings, fmt.Sprintf("%s: German copy uses the retired word %q; "+
					"docs/reference/ui-copy-style-de.md names the word to use: %q", one.at, retired[i], one.text))
			}
		}
	}
	return findings
}

func serverGermanStrings(t *testing.T) []germanString {
	t.Helper()
	var found []germanString
	for _, root := range germanCopyRoots {
		err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if unauthoredDir(entry.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			found = append(found, germanStringsIn(gatekit.SourceFileSet(), filepath.ToSlash(path), file)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s for German copy: %v", root, err)
		}
	}
	return found
}

const (
	shapeKey  = "key"
	shapeCase = "case"
	shapeRow  = "row"
)

func germanStringsIn(fset *token.FileSet, path string, file *ast.File) []germanString {
	var found []germanString
	var take func(shape string, node ast.Node)
	take = func(shape string, node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				// A map key inside German copy is a code, not copy.
				take(shape, kv.Value)
				return false
			}
			if text, ok := germanLiteral(n); ok {
				found = append(found, germanString{
					at: fmt.Sprintf("%s:%d", path, fset.Position(n.Pos()).Line), shape: shape, text: text,
				})
			}
			return true
		})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if namesGerman(node.Key) {
				take(shapeKey, node.Value)
				return false
			}
		case *ast.CaseClause:
			if slices.ContainsFunc(node.List, namesGerman) {
				for _, stmt := range node.Body {
					take(shapeCase, stmt)
				}
				return false
			}
		case *ast.CallExpr:
			for _, german := range rowGerman(node.Args) {
				take(shapeRow, german)
			}
		case *ast.CompositeLit:
			for _, german := range rowGerman(node.Elts) {
				take(shapeRow, german)
			}
		}
		return true
	})
	return found
}

func namesGerman(expr ast.Expr) bool {
	switch key := expr.(type) {
	case *ast.Ident:
		return key.Name == "De" || key.Name == "German" || key.Name == "german"
	case *ast.SelectorExpr:
		return key.Sel.Name == "German"
	}
	return false
}

// rowGerman answers the middle of each run of three string literals whose
// third carries a letter only Vietnamese writes.
func rowGerman(list []ast.Expr) []ast.Expr {
	var german []ast.Expr
	for i := 0; i+2 < len(list); i++ {
		_, english := joinedLiteral(list[i])
		_, middle := joinedLiteral(list[i+1])
		third, vietnamese := joinedLiteral(list[i+2])
		if english && middle && vietnamese && strings.ContainsAny(third, vietnameseOnlyRunes) {
			german = append(german, list[i+1])
			i += 2
		}
	}
	return german
}

// joinedLiteral reads a long sentence split over several literals with +.
func joinedLiteral(expr ast.Expr) (string, bool) {
	if sum, ok := expr.(*ast.BinaryExpr); ok && sum.Op == token.ADD {
		left, leftOK := joinedLiteral(sum.X)
		right, rightOK := joinedLiteral(sum.Y)
		return left + right, leftOK && rightOK
	}
	return germanLiteral(expr)
}

func germanLiteral(node ast.Node) (string, bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}
