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
//   - the middle of an English, German, Vietnamese row of string literals;
//   - a key "de", and the other strings of a record whose language is "de";
//   - each line of an embedded file in a folder named de, or named *.de.*.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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
	germanRecordFloor   = 5
	germanFileFloor     = 300
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
	if len(found) < germanStringFloor || shapes[shapeRow] < germanRowFloor || shapes[shapeCase] < germanCaseFloor ||
		shapes[shapeRecord] < germanRecordFloor || shapes[shapeFile] < germanFileFloor {
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
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "documents", "de"), 0o755); err != nil {
		t.Fatalf("making the planted document folder: %v", err)
	}
	for name, body := range map[string]string{"de/vertrag.md": "# Vertrag\n\nDie Firma speichert.\n", "en/contract.md": "The Firma"} {
		path := filepath.Join(dir, "documents", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("making %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("planting %s: %v", path, err)
		}
	}
	planted := `package planted

//go:embed documents/de/*.md documents/en/*.md
var files embed.FS

var stages = map[string]map[string]string{"Won": {"de": "Firma", "vi": "Thắng"}}
var entry = documentEntry{"documents/de/vertrag.md", "Neue Firma", "de"}
var keyed = document{Title: "Firma", Language: "de"}

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
	path := filepath.Join(dir, "planted.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, planted, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the planted shapes: %v", err)
	}
	found, err := germanStringsIn(token.NewFileSet(), path, file)
	if err != nil {
		t.Fatalf("reading the planted embedded files: %v", err)
	}
	findings := retiredWordFindings(found, []string{"Firma"})
	if len(findings) != 10 {
		t.Errorf("the walk finds %d of the 10 planted retired words: %v", len(findings), findings)
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
	stated := retiredCount.FindStringSubmatch(string(page))
	if stated == nil {
		t.Fatalf("%s does not state how many words its Never column retires", germanStylePage)
	}
	if strconv.Itoa(len(words)) != stated[1] {
		t.Fatalf("read %d retired words from the Never column, and the page states %s; "+
			"either the parse lost a row or the stated count is stale", len(words), stated[1])
	}
	return words
}

var (
	neverWord    = regexp.MustCompile("`([^`]+)`")
	retiredCount = regexp.MustCompile(`The Never column\s+retires\s+(\d+)\s+words`)
)

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
			file, err := gatekit.ParseFile(path, parser.SkipObjectResolution|parser.ParseComments)
			if err != nil {
				return err
			}
			inFile, err := germanStringsIn(gatekit.SourceFileSet(), filepath.ToSlash(path), file)
			found = append(found, inFile...)
			return err
		})
		if err != nil {
			t.Fatalf("walking %s for German copy: %v", root, err)
		}
	}
	return found
}

const (
	shapeKey    = "key"
	shapeCase   = "case"
	shapeRow    = "row"
	shapeRecord = "record"
	shapeFile   = "file"
)

func germanStringsIn(fset *token.FileSet, path string, file *ast.File) ([]germanString, error) {
	var found []germanString
	var take func(shape string, node ast.Node)
	take = func(shape string, node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				// A map key inside German copy is a code, not copy.
				take(shape, kv.Value)
				return false
			}
			lit, isLit := n.(*ast.BasicLit)
			if !isLit {
				return true
			}
			if text, ok := literalText(lit); ok {
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
			for _, german := range recordGerman(node.Elts) {
				take(shapeRecord, german)
			}
		}
		return true
	})
	files, err := embeddedGerman(path, file)
	return append(found, files...), err
}

// recordGerman answers the other string fields of a record that names its
// language "de", positionally or under a key.
func recordGerman(elements []ast.Expr) []ast.Expr {
	values := make([]ast.Expr, len(elements))
	german := false
	for i, element := range elements {
		values[i] = element
		if kv, ok := element.(*ast.KeyValueExpr); ok {
			values[i] = kv.Value
		}
		text, ok := literalText(values[i])
		german = german || ok && text == "de"
	}
	if !german {
		return nil
	}
	return slices.DeleteFunc(values, func(value ast.Expr) bool {
		text, ok := literalText(value)
		return !ok || text == "de"
	})
}

// embeddedGerman reads every line of the German files a //go:embed directive
// takes. A file is German in a folder named de, or under a name like terms.de.md.
func embeddedGerman(path string, file *ast.File) ([]germanString, error) {
	var found []germanString
	for _, group := range file.Comments {
		for _, comment := range group.List {
			patterns, ok := strings.CutPrefix(comment.Text, "//go:embed ")
			if !ok {
				continue
			}
			for pattern := range strings.FieldsSeq(patterns) {
				lines, err := embeddedGermanLines(filepath.Join(filepath.Dir(path), filepath.FromSlash(pattern)))
				if err != nil {
					return found, fmt.Errorf("reading the embed pattern %q in %s: %w", pattern, path, err)
				}
				found = append(found, lines...)
			}
		}
	}
	return found, nil
}

// embeddedGermanLines globs one pattern, and walks a matched folder the way
// //go:embed takes it whole.
func embeddedGermanLines(pattern string) ([]germanString, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var found []germanString
	for _, match := range matches {
		err := gatekit.WalkDir(match, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !germanFile(path) {
				return walkErr
			}
			lines, err := germanFileLines(path)
			found = append(found, lines...)
			return err
		})
		if err != nil {
			return found, err
		}
	}
	return found, nil
}

func germanFile(path string) bool {
	if filepath.Base(filepath.Dir(path)) == "de" {
		return true
	}
	parts := strings.FieldsFunc(filepath.Base(path), func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	return slices.Contains(parts, "de")
}

func germanFileLines(path string) ([]germanString, error) {
	body, err := gatekit.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the embedded German file %s: %w", path, err)
	}
	var found []germanString
	for number, line := range strings.Split(string(body), "\n") {
		found = append(found, germanString{
			at: fmt.Sprintf("%s:%d", filepath.ToSlash(path), number+1), shape: shapeFile, text: line,
		})
	}
	return found, nil
}

func namesGerman(expr ast.Expr) bool {
	switch key := expr.(type) {
	case *ast.Ident:
		return key.Name == "De" || key.Name == "German" || key.Name == "german"
	case *ast.SelectorExpr:
		return key.Sel.Name == "German"
	case *ast.BasicLit:
		text, ok := literalText(key)
		return ok && text == "de"
	}
	return false
}

// rowGerman answers the middle of each run of three string literals whose
// third carries a letter only Vietnamese writes.
func rowGerman(list []ast.Expr) []ast.Expr {
	var german []ast.Expr
	for i := 0; i+2 < len(list); i++ {
		_, english := literalText(list[i])
		_, middle := literalText(list[i+1])
		third, vietnamese := literalText(list[i+2])
		if english && middle && vietnamese && strings.ContainsAny(third, vietnameseOnlyRunes) {
			german = append(german, list[i+1])
			i += 2
		}
	}
	return german
}

// literalText reads a string literal, or a sentence split over literals with +.
// A literal on its own is the unit the walk judges, so a part of a
// concatenation is read once, as itself.
func literalText(node ast.Node) (string, bool) {
	switch node.(type) {
	case *ast.BasicLit, *ast.BinaryExpr:
		return gatekit.StringExpr(node, nil, gatekit.FoldStrict)
	}
	return "", false
}
