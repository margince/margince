// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The text census that the syntax scan of langcopylanguagetables_test.go
// answers to. It declares no Test function, so it owes no //gate:kind line.

package gates

import (
	"bytes"
	"cmp"
	"go/scanner"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// containerPrefix spells the map, slice, array and pointer types a language
// map may sit in, whose literals elide its type.
const containerPrefix = `(?:map\[[^\[\]]*\]|\[[^\[\]]*\]|\*)*`

// typeParams spells the type parameters of a generic type or its instance.
const typeParams = `(?:\[[^\]]*\])?`

// valueType spells a map's value type up to the { of a literal, so a
// conversion such as map[textlang.Lang]string(nil) is no literal.
const valueType = `(?:\*|\[[^\]]*\]|map\[[^\]]*\])*` +
	`(?:struct\s*\{[^{}]*\}|interface\s*\{[^{}]*\}|func\s*\([^()]*\)[\w.\s*\[\]]*?|[\w.]+` + typeParams + `)\s*\{`

var namedType = regexp.MustCompile(`^type\s+(\w+)\s+(?:=\s*)?(\w+)\s*$`)

// languageSpelling is how one directory's text spells a language-keyed map
// and the named types that hold one.
type languageSpelling struct {
	keyedMap string
	mapTypes []string
}

// literal matches a literal of a language map, of a named type that holds one,
// or of a container whose elements are either.
func (s languageSpelling) literal() *regexp.Regexp {
	spelled := []string{s.keyedMap + valueType}
	for _, name := range s.mapTypes {
		spelled = append(spelled, name+typeParams+`\{`)
	}
	return regexp.MustCompile(`(?:[=:]|return)\s*` + containerPrefix + `(?:` + strings.Join(spelled, "|") + `)`)
}

// languageTexts reads each non-test Go file under root as plain text. It skips
// no directory, so a directory the syntax scan stops reading fails.
func languageTexts(t *testing.T, root string) map[string][]byte {
	t.Helper()
	texts := map[string][]byte{}
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		text, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		texts[filepath.ToSlash(path)] = text
		return nil
	})
	if err != nil {
		t.Fatalf("reading the module as text for language-keyed literals: %v", err)
	}
	return texts
}

// spelledLiteral is where the text census reads a language-keyed literal:
// the byte offset of the = : or return before it, and that offset's line.
type spelledLiteral struct {
	path         string
	offset, line int
}

// languageLiteralsIn is the census the syntax scan answers to. It holds one
// entry per literal, so a table the scan reads cannot stand in for its neighbour.
func languageLiteralsIn(texts map[string][]byte) []spelledLiteral {
	lexed := map[string]languageText{}
	for path, text := range texts {
		lexed[path] = lexLanguageText(text)
	}
	spellings, literals := languageSpellings(lexed), map[string]*regexp.Regexp{}
	var found []spelledLiteral
	for path, file := range lexed {
		dir := filepath.Dir(path)
		if literals[dir] == nil {
			literals[dir] = spellings[dir].literal()
		}
		for _, match := range literals[dir].FindAllIndex(file.code, -1) {
			if file.declaresTypeAt(match[0]) {
				continue
			}
			line := bytes.Count(file.code[:match[0]], []byte("\n")) + 1
			found = append(found, spelledLiteral{path: path, offset: match[0], line: line})
		}
	}
	slices.SortFunc(found, func(a, b spelledLiteral) int {
		return cmp.Or(strings.Compare(a.path, b.path), cmp.Compare(a.offset, b.offset))
	})
	return found
}

// unreadLiterals answers the literals that lie in no declaration the syntax
// scan read as a table, by the byte ranges it read in each file.
func unreadLiterals(literals []spelledLiteral, tables map[string][][2]int) []spelledLiteral {
	var unread []spelledLiteral
	for _, literal := range literals {
		if !slices.ContainsFunc(tables[literal.path], func(span [2]int) bool { return within(span, literal.offset) }) {
			unread = append(unread, literal)
		}
	}
	return unread
}

func within(span [2]int, offset int) bool { return span[0] <= offset && offset < span[1] }

func languageSpellings(lexed map[string]languageText) map[string]languageSpelling {
	byDir := map[string][]languageText{}
	for path, file := range lexed {
		byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], file)
	}
	spellings := map[string]languageSpelling{}
	for dir, files := range byDir {
		spellings[dir] = spellingIn(files)
	}
	return spellings
}

// spellingIn reads every import alias of textlang and every name declared for
// textlang.Lang before the map types, since any file may hold either.
func spellingIn(files []languageText) languageSpelling {
	qualifiers, namedAs := []string{"textlang"}, map[string]string{}
	var declarations []string
	for _, file := range files {
		qualifiers = append(qualifiers, file.qualifiers...)
		declarations = append(declarations, file.typeDeclarations()...)
	}
	for _, declaration := range declarations {
		if named := namedType.FindStringSubmatch(declaration); named != nil {
			namedAs[named[1]] = named[2]
		}
	}
	declared := `^type\s+(\w+)` + typeParams + `\s+(?:=\s*)?`
	lang := `(?:` + strings.Join(qualifiers, "|") + `)\.Lang\b`
	keys := declaredAs(declarations, declared+lang, namedAs)
	keyedMap := `map\[(?:` + strings.Join(append([]string{lang}, keys...), "|") + `)\]`
	return languageSpelling{keyedMap: keyedMap, mapTypes: declaredAs(declarations, declared+containerPrefix+keyedMap, namedAs)}
}

// declaredAs answers the types a declaration matching pattern names, and each
// type declared as one of them.
func declaredAs(declarations []string, pattern string, namedAs map[string]string) []string {
	declaration, declared := regexp.MustCompile(pattern), map[string]bool{}
	for _, text := range declarations {
		if match := declaration.FindStringSubmatch(text); match != nil {
			declared[match[1]] = true
		}
	}
	withNamedChains(declared, namedAs)
	return slices.Sorted(maps.Keys(declared))
}

// languageText is one file's text with its comments and string contents
// blanked, which keeps every byte offset. It keeps the byte range of each type
// spec, grouped or not, and each name the file imports textlang under.
type languageText struct {
	code       []byte
	typeSpecs  [][2]int
	qualifiers []string
}

// typeDeclarations spells each type spec as a declaration of its own.
func (l languageText) typeDeclarations() []string {
	declarations := make([]string, len(l.typeSpecs))
	for i, spec := range l.typeSpecs {
		declarations[i] = "type " + string(l.code[spec[0]:spec[1]])
	}
	return declarations
}

// declaresTypeAt reports whether offset falls in a type spec, where a type
// alias spells "= map[" and declares no literal.
func (l languageText) declaresTypeAt(offset int) bool {
	return slices.ContainsFunc(l.typeSpecs, func(span [2]int) bool { return within(span, offset) })
}

// lexLanguageText reads tokens and not syntax, so the census stays a second
// reading of the file that shares no parse with the syntax scan.
func lexLanguageText(text []byte) languageText {
	file := token.NewFileSet().AddFile("", -1, len(text))
	var lexer scanner.Scanner
	lexer.Init(file, text, nil, scanner.ScanComments)
	lexed, specs, alias := languageText{code: bytes.Clone(text)}, typeSpecReader{}, ""
	for {
		pos, tok, lit := lexer.Scan()
		start := file.Offset(pos)
		switch tok {
		case token.EOF:
			return lexed
		case token.COMMENT:
			blankComment(lexed.code, start)
			continue
		case token.STRING:
			if path, err := strconv.Unquote(lit); err == nil && path == textlangImport && alias != "" {
				lexed.qualifiers = append(lexed.qualifiers, alias)
			}
			blankString(lexed.code, start, lit)
		}
		if spec, ended := specs.read(tok, start); ended {
			lexed.typeSpecs = append(lexed.typeSpecs, spec)
		}
		alias = ""
		if tok == token.IDENT {
			alias = lit
		}
	}
}

// blankComment overwrites a comment. The lexer strips a comment's carriage
// returns, so its end is found in the text.
func blankComment(code []byte, start int) {
	end := len(code)
	if bytes.HasPrefix(code[start:], []byte("//")) {
		if newline := bytes.IndexByte(code[start:], '\n'); newline >= 0 {
			end = start + newline
		}
	} else if closing := bytes.Index(code[start+2:], []byte("*/")); closing >= 0 {
		end = start + 2 + closing + 2
	}
	blank(code[start:end])
}

// blankString overwrites what a string literal holds and keeps its quotes. A
// raw string's carriage returns are stripped from lit too.
func blankString(code []byte, start int, lit string) {
	end := start + len(lit) - 1
	if code[start] == '`' {
		end = start + 1 + bytes.IndexByte(code[start+1:], '`')
	}
	blank(code[start+1 : end])
}

// blank overwrites text with spaces and keeps its newlines, so offsets and
// line numbers stay as they were.
func blank(text []byte) {
	for i := range text {
		if text[i] != '\n' {
			text[i] = ' '
		}
	}
}

// typeSpecReader follows a type declaration from its name to the semicolon or
// closing bracket at its own depth. Only a name or ( after the keyword starts
// one, so the type keyword of a type switch declares nothing.
type typeSpecReader struct {
	afterType, grouped, inSpec bool
	start, depth               int
}

func (r *typeSpecReader) read(tok token.Token, offset int) (spec [2]int, ended bool) {
	switch {
	case r.inSpec:
		return r.readSpec(tok, offset)
	case r.afterType:
		r.afterType, r.grouped = false, tok == token.LPAREN
		r.inSpec, r.start, r.depth = tok == token.IDENT, offset, 0
	case r.grouped:
		r.grouped = tok != token.RPAREN
		r.inSpec, r.start, r.depth = tok == token.IDENT, offset, 0
	case tok == token.TYPE:
		r.afterType = true
	}
	return spec, false
}

func (r *typeSpecReader) readSpec(tok token.Token, offset int) (spec [2]int, ended bool) {
	switch tok {
	case token.LPAREN, token.LBRACK, token.LBRACE:
		r.depth++
	case token.RPAREN, token.RBRACK, token.RBRACE:
		if r.depth > 0 {
			r.depth--
			return spec, false
		}
		// A group's ) ends it; any other closer belongs to the code around the spec.
		r.inSpec, r.grouped = false, false
		return [2]int{r.start, offset}, true
	case token.SEMICOLON:
		if r.depth == 0 {
			r.inSpec = false
			return [2]int{r.start, offset}, true
		}
	}
	return spec, false
}
