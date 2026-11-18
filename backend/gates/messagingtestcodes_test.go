// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The messaging-rules registry panics when one code is registered twice in a
// process. Every test file of a Go package, under every build tag, lands in one
// test binary. So two test files that register one code pass alone and panic
// when a lane builds them together. This census reads every registration a
// test writes with a literal code and fails a code one package uses twice.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// registeredCode is one literal jurisdiction code a test registers.
type registeredCode struct {
	code  string
	where token.Pos
}

func TestNoTestPackageRegistersOneMessagingCodeTwice(t *testing.T) {
	t.Parallel()
	byPackage := map[string][]*ast.File{}
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		byPackage[filepath.Dir(path)] = append(byPackage[filepath.Dir(path)], file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for dir, files := range byPackage {
		codes := registeredCodes(files)
		total += len(codes)
		for _, clash := range clashes(codes) {
			t.Errorf("%s: test code %q is registered at %s and at %s; one test binary panics on the second. "+
				"Pick a code no other test in the package registers",
				dir, clash[0].code, gatekit.SourceFileSet().Position(clash[0].where), gatekit.SourceFileSet().Position(clash[1].where))
		}
	}
	// The consent and compose test packages register 21 codes this way.
	if total < 20 {
		t.Fatalf("the census read %d registered codes; the registrations moved or changed shape", total)
	}
}

func TestATestCodeRegisteredTwiceIsFound(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe_test.go", `package probe
func TestOne(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "aa", Version: 1}) }
var packs = sync.OnceValue(func() int {
	rules := shipped()
	rules.Jurisdiction = "aa"
	messagingrules.Register(rules)
	return 0
})
var unrelated = sync.OnceValue(func() messagingrules.Rules { return messagingrules.Rules{Jurisdiction: "aa"} })
var other = sync.OnceValue(func() int {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "cc"})
	subjectrightsrules.Register(subjectrightsrules.Rules{Jurisdiction: "cc"})
	return 0
})
func TestTwo(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "bb", Version: 1}) }
const shared = "bb"
func TestThree(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: shared}) }`,
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	codes := registeredCodes([]*ast.File{file})
	if len(codes) != 5 {
		t.Fatalf("the probe registers 5 messaging codes and the census read %d: %+v", len(codes), codes)
	}
	found := clashes(codes)
	if len(found) != 2 || found[0][0].code != "aa" || found[1][0].code != "bb" {
		t.Errorf("want the clashes on \"aa\" and on the constant \"bb\", got %+v", found)
	}
}

// registeredCodes reads the codes the files register: a Jurisdiction keyed in
// a Register call, and, inside a func literal that calls Register, one keyed in
// a messagingrules.Rules literal or assigned to a variable it registers. A code is a string literal or a constant the
// package declares as one. A place both rules reach is counted once.
func registeredCodes(files []*ast.File) []registeredCode {
	r := codeReader{consts: codeConsts(files), seen: map[token.Pos]string{}}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if isRegisterCall(node) {
					for _, arg := range node.Args {
						r.jurisdictions(arg)
					}
				}
			case *ast.FuncLit:
				if callsRegister(node.Body) {
					r.jurisdictions(node.Body)
				}
			}
			return true
		})
	}
	out := make([]registeredCode, 0, len(r.seen))
	for where, code := range r.seen {
		out = append(out, registeredCode{code: code, where: where})
	}
	slices.SortFunc(out, func(a, b registeredCode) int { return int(a.where - b.where) })
	return out
}

// isRegisterCall reports a call of messagingrules.Register.
func isRegisterCall(call *ast.CallExpr) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "Register" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == "messagingrules"
}

func callsRegister(body ast.Node) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && isRegisterCall(call) {
			found = true
		}
		return !found
	})
	return found
}

// codeConsts maps each constant the files declare with a string literal to its
// value, inside functions too, which stringConsts does not read. A name
// declared twice with two values maps to neither.
func codeConsts(files []*ast.File) map[string]string {
	out, ambiguous := map[string]string{}, map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, isDecl := n.(*ast.GenDecl)
			if !isDecl || decl.Tok != token.CONST {
				return true
			}
			for _, spec := range decl.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					text, isString := gatekit.LiteralText(value.Values[i])
					if have, declared := out[name.Name]; isString && declared && have != text {
						ambiguous[name.Name] = true
					} else if isString {
						out[name.Name] = text
					}
				}
			}
			return true
		})
	}
	for name := range ambiguous {
		delete(out, name)
	}
	return out
}

// codeReader collects the codes one package registers, by where they are written.
type codeReader struct {
	consts map[string]string
	seen   map[token.Pos]string
}

// jurisdictions adds each code under node that a messagingrules.Rules literal
// keys as its Jurisdiction, or that is assigned to the Jurisdiction of a
// variable node passes to Register.
func (r codeReader) jurisdictions(node ast.Node) {
	registered := registeredNames(node)
	ast.Inspect(node, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.CompositeLit:
			if !isRulesLiteral(expr) {
				return true
			}
			for _, elt := range expr.Elts {
				if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
					if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Jurisdiction" {
						r.add(kv.Value)
					}
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range expr.Lhs {
				sel, isSel := lhs.(*ast.SelectorExpr)
				if !isSel || sel.Sel.Name != "Jurisdiction" || i >= len(expr.Rhs) {
					continue
				}
				if owner, isIdent := sel.X.(*ast.Ident); isIdent && registered[owner.Name] {
					r.add(expr.Rhs[i])
				}
			}
		}
		return true
	})
}

func isRulesLiteral(lit *ast.CompositeLit) bool {
	sel, isSel := lit.Type.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "Rules" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == "messagingrules"
}

// registeredNames are the variables node passes to messagingrules.Register.
func registeredNames(node ast.Node) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && isRegisterCall(call) {
			for _, arg := range call.Args {
				if ident, isIdent := arg.(*ast.Ident); isIdent {
					out[ident.Name] = true
				}
			}
		}
		return true
	})
	return out
}

func (r codeReader) add(expr ast.Expr) {
	if code, isString := gatekit.LiteralText(expr); isString {
		r.seen[expr.Pos()] = code
		return
	}
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		if code, isConst := r.consts[ident.Name]; isConst {
			r.seen[expr.Pos()] = code
		}
	}
}

// clashes pairs each code with the first place the package registered it.
func clashes(codes []registeredCode) [][2]registeredCode {
	first := map[string]registeredCode{}
	var out [][2]registeredCode
	for _, c := range codes {
		if earlier, seen := first[c.code]; seen {
			out = append(out, [2]registeredCode{earlier, c})
			continue
		}
		first[c.code] = c
	}
	return out
}
