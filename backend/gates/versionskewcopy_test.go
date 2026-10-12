// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// Every apperrors.VersionSkewError message is a sentence its reader can act on.
//
// The message reaches a REST client and an MCP agent verbatim. It opens with a
// capital, carries no em dash, and keeps each sentence within the word limit.
//
// Narration and jargon need a human reader. A message built at run time fails as unreadable.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	versionSkewType          = "VersionSkewError"
	versionSkewSentenceWords = 25
	emDash                   = "—"
)

// skewMessage is one VersionSkewError construction and the message it sets.
type skewMessage struct {
	where    string
	text     string
	readable bool
}

func TestEveryVersionSkewMessageIsAReadersSentence(t *testing.T) {
	t.Parallel()
	var sites []skewMessage
	written := 0
	consts := map[string]map[string]string{}
	for _, tracked := range trackedFiles(t) {
		rel, inModule := strings.CutPrefix(tracked.path, "backend/")
		if !inModule || tracked.symlink || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		src, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		mentions := bytes.Count(src, []byte(versionSkewType+"{"))
		if mentions == 0 {
			continue
		}
		written += mentions
		file, err := gatekit.ParseFile(rel, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		dir := filepath.Dir(rel)
		if consts[dir] == nil {
			consts[dir] = gatekit.PackageStringConstants(t, dir)
		}
		sites = append(sites, versionSkewMessages(gatekit.SourceFileSet(), file, consts[dir])...)
	}
	// The text count is the census the walk must match: a construction the AST
	// walk does not recognise would otherwise leave the tree reading clean.
	if written == 0 || len(sites) != written {
		t.Fatalf("the walk found %d %s constructions where the source spells %d: a construction shape "+
			"went unrecognised, so its message was never judged", len(sites), versionSkewType, written)
	}
	for _, site := range sites {
		for _, fault := range site.faults() {
			t.Errorf("%s: this version-skew message reaches its reader verbatim and %s (%q)", site.where, fault, site.text)
		}
	}
}

// The walk is planted with every shape it claims to judge. A change that
// narrows it then fails here instead of reading clean.
func TestVersionSkewCopyGateFaultsEveryPlantedShape(t *testing.T) {
	t.Parallel()
	const planted = `package planted

const advice = "Read it again."

func lowercase() error { return &apperrors.VersionSkewError{Message: "the record moved. " + advice} }
func dashed() error { return &apperrors.VersionSkewError{Message: "The record moved — read it again."} }
func long(n int) error {
	return &apperrors.VersionSkewError{Message: fmt.Sprintf("The record %d moved while this call ran and "+
		"so nothing at all was changed by it, which means you should read the record and try again now.", n)}
}
func computed(m string) error { return &apperrors.VersionSkewError{Message: m} }
func unnamed() error { return &VersionSkewError{} }
func reader() error { return &apperrors.VersionSkewError{Message: "The record moved. " + advice} }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "planted.go", planted, 0)
	if err != nil {
		t.Fatalf("parsing the planted package: %v", err)
	}
	sites := versionSkewMessages(fset, file, map[string]string{"advice": "Read it again."})
	faulted := []bool{true, true, true, true, true, false}
	if len(sites) != len(faulted) {
		t.Fatalf("the walk found %d planted constructions, want %d", len(sites), len(faulted))
	}
	for i, site := range sites {
		if got := len(site.faults()) > 0; got != faulted[i] {
			t.Errorf("%s: faulted = %v, want %v (%q)", site.where, got, faulted[i], site.text)
		}
	}
}

func versionSkewMessages(fset *token.FileSet, file *ast.File, consts map[string]string) []skewMessage {
	var out []skewMessage
	ast.Inspect(file, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit || !namesVersionSkew(lit.Type) {
			return true
		}
		pos := fset.Position(lit.Pos())
		site := skewMessage{where: fmt.Sprintf("%s:%d", pos.Filename, pos.Line)}
		if message := messageExpr(lit); message != nil {
			site.text, site.readable = gatekit.StringExpr(message, consts, gatekit.FoldStrict)
		}
		out = append(out, site)
		return true
	})
	return out
}

func namesVersionSkew(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name == versionSkewType
	case *ast.SelectorExpr:
		return typ.Sel.Name == versionSkewType
	}
	return false
}

// messageExpr is the expression a construction sets Message to, unwrapped from
// the fmt.Sprintf whose format carries its words.
func messageExpr(lit *ast.CompositeLit) ast.Expr {
	for _, element := range lit.Elts {
		value := element
		if pair, keyed := element.(*ast.KeyValueExpr); keyed {
			if key, isIdent := pair.Key.(*ast.Ident); !isIdent || key.Name != "Message" {
				continue
			}
			value = pair.Value
		}
		call, isCall := value.(*ast.CallExpr)
		if !isCall || len(call.Args) == 0 {
			return value
		}
		if fun, isSelector := call.Fun.(*ast.SelectorExpr); isSelector && fun.Sel.Name == "Sprintf" {
			return call.Args[0]
		}
		return value
	}
	return nil
}

func (m skewMessage) faults() []string {
	if !m.readable {
		return []string{"cannot be read here; build it from literals and constants so its words can be judged"}
	}
	var faults []string
	if first, _ := utf8.DecodeRuneInString(m.text); !unicode.IsUpper(first) {
		faults = append(faults, "does not open with a capital letter")
	}
	if strings.Contains(m.text, emDash) {
		faults = append(faults, "carries an em dash; end the sentence instead")
	}
	fields := strings.Fields(m.text)
	words := 0
	for i, word := range fields {
		words++
		if !strings.ContainsAny(word[len(word)-1:], ".?!") && i < len(fields)-1 {
			continue
		}
		if words > versionSkewSentenceWords {
			faults = append(faults, fmt.Sprintf("runs a sentence to %d words, over %d", words, versionSkewSentenceWords))
		}
		words = 0
	}
	return faults
}
