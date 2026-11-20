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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
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
	codes := 0
	var tables []string
	for dir, files := range byPackage {
		codes += len(registeredCodes(files))
		tables = append(tables, readableTables(files)...)
		for _, finding := range codeFindings(dir, files) {
			t.Error(finding)
		}
	}
	// The consent and compose test packages register 19 codes this way. The
	// shipped packs register under codes shippedPackCodeRanges hands out.
	if codes < 18 {
		t.Fatalf("the census read %d registered codes; the registrations moved or changed shape", codes)
	}
	for _, missing := range missingTables(tables) {
		t.Errorf("no test package declares %s as a table of ranges this gate reads; the allocator moved or changed "+
			"shape, and the codes it hands out are unchecked", missing)
	}
}

// readableTables names each handedOutTables declaration the files hold in the
// shape handedOut reads: a composite literal of range entries.
func readableTables(files []*ast.File) []string {
	var out []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, isSpec := n.(*ast.ValueSpec)
			if !isSpec || len(spec.Names) != 1 || !slices.Contains(handedOutTables, spec.Names[0].Name) || len(spec.Values) != 1 {
				return true
			}
			if table, isTable := spec.Values[0].(*ast.CompositeLit); isTable && slices.ContainsFunc(table.Elts,
				func(e ast.Expr) bool { _, isEntry := e.(*ast.CompositeLit); return isEntry }) {
				out = append(out, spec.Names[0].Name)
			}
			return false
		})
	}
	return out
}

// missingTables are the handedOutTables names found lists none of.
func missingTables(found []string) []string {
	var out []string
	for _, name := range handedOutTables {
		if !slices.Contains(found, name) {
			out = append(out, name)
		}
	}
	return out
}

// codeFindings names each code two registrations in one package share, and
// each literal code inside a range the package's allocator hands out.
func codeFindings(dir string, files []*ast.File) []string {
	positions := gatekit.SourceFileSet()
	codes := registeredCodes(files)
	var out []string
	for _, clash := range clashes(codes) {
		out = append(out, fmt.Sprintf("%s: test code %q is registered at %s and at %s; one test binary panics on the "+
			"second. Pick a code no other test in the package registers",
			dir, clash[0].code, positions.Position(clash[0].where), positions.Position(clash[1].where)))
	}
	ranges := handedOut(files)
	for _, c := range codes {
		if slices.ContainsFunc(ranges, func(r codeRange) bool { return r.holds(c.code) }) {
			out = append(out, fmt.Sprintf("%s: test code %q at %s is one %s hands out; pick a code outside it",
				dir, c.code, positions.Position(c.where), strings.Join(handedOutTables, " or ")))
		}
	}
	for i, a := range ranges {
		for _, b := range ranges[i+1:] {
			if a.overlaps(b) {
				out = append(out, fmt.Sprintf("%s: two handed-out ranges both hold codes under %q; one code would be "+
					"handed out twice. Move one range", dir, string(a.prefix)))
			}
		}
	}
	return out
}

// handedOutTables are the declarations that state which codes a package's
// run-time allocators hand out, as prefix, first and last rune. One is for test
// rule sets and one for the shipped packs.
var handedOutTables = []string{"testCodeRanges", "shippedPackCodeRanges"}

// codeRange is one prefix and the second letters handed out under it.
type codeRange struct {
	prefix, first, last rune
	// any marks an entry the gate could not read, which holds every code.
	any bool
}

func (r codeRange) overlaps(o codeRange) bool {
	return r.any || o.any || (r.prefix == o.prefix && r.first <= o.last && o.first <= r.last)
}

func (r codeRange) holds(code string) bool {
	if r.any {
		return true
	}
	runes := []rune(code)
	return len(runes) == 2 && runes[0] == r.prefix && runes[1] >= r.first && runes[1] <= r.last
}

// handedOut reads the ranges the files declare in handedOutTables.
func handedOut(files []*ast.File) []codeRange {
	var out []codeRange
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, isSpec := n.(*ast.ValueSpec)
			if !isSpec || len(spec.Names) != 1 || !slices.Contains(handedOutTables, spec.Names[0].Name) || len(spec.Values) != 1 {
				return true
			}
			if table, isTable := spec.Values[0].(*ast.CompositeLit); isTable {
				for _, elt := range table.Elts {
					if entry, isEntry := elt.(*ast.CompositeLit); isEntry {
						out = append(out, rangeOf(entry))
					}
				}
			}
			return false
		})
	}
	return out
}

// rangeOf reads one table entry. An entry it cannot read whole is answered
// as covering every code, so it fails loudly rather than checking nothing.
func rangeOf(entry *ast.CompositeLit) codeRange {
	var r codeRange
	for _, elt := range entry.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		lit, isLit := kv.Value.(*ast.BasicLit)
		if !isIdent || !isLit || lit.Kind != token.CHAR {
			continue
		}
		value, _, _, err := strconv.UnquoteChar(strings.Trim(lit.Value, "'"), '\'')
		if err != nil {
			continue
		}
		switch key.Name {
		case "prefix":
			r.prefix = value
		case "first":
			r.first = value
		case "last":
			r.last = value
		}
	}
	if r.prefix == 0 || r.first == 0 || r.last == 0 {
		return codeRange{any: true}
	}
	return r
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

// registeredCodes reads the codes the files register. One is a Jurisdiction
// keyed in a Register call. Inside a func literal that calls Register, it is
// also one keyed in a messagingrules.Rules literal or assigned to a variable
// the literal registers. A code is a string literal or a string constant, and
// a place both rules reach is counted once.
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
// keys as its Jurisdiction. It also adds each code assigned to the Jurisdiction
// of a variable that node passes to Register.
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

// A literal inside the allocator's range is found, and one outside it is not.
func TestATestCodeTheAllocatorHandsOutIsFound(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe_test.go", `package probe
var testCodeRanges = [...]testCodeRange{{prefix: 'q', first: 'm', last: 'z'}, {prefix: 'x', first: 'a', last: 'z'}}
func TestOne(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "qv"}) }
func TestTwo(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "qa"}) }
func TestThree(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "pv"}) }`,
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	findings := codeFindings("probe", []*ast.File{file})
	if len(findings) != 1 || !strings.Contains(findings[0], `"qv"`) {
		t.Errorf("want one finding, on \"qv\": %q", findings)
	}
}

func TestALiteralCodeInsideAHandedOutRangeIsFound(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe_test.go", `package probe
var testCodeRanges = [...]r{{prefix: 'q', first: 'm', last: 'z'}}
var shippedPackCodeRanges = [...]r{{prefix: 'm', first: 'a', last: 'z'}, {prefix: 'q', first: 'a', last: 'n'}}
func TestOne(t *testing.T) { messagingrules.Register(messagingrules.Rules{Jurisdiction: "mb", Version: 1}) }`,
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	findings := strings.Join(codeFindings("probe", []*ast.File{file}), "\n")
	if !strings.Contains(findings, `test code "mb"`) {
		t.Errorf("a literal code inside shippedPackCodeRanges passed: %s", findings)
	}
	if !strings.Contains(findings, `under "q"`) {
		t.Errorf("two tables handing out the same q codes passed: %s", findings)
	}
}

func TestEachAllocatorTableMustBeFoundByName(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"one table missing": `package probe
var testCodeRanges = [...]r{{prefix: 'q', first: 'm', last: 'z'}}`,
		"one table in a shape the gate cannot read": `package probe
var testCodeRanges = [...]r{{prefix: 'q', first: 'm', last: 'z'}}
var shippedPackCodeRanges = rangesFrom("m")`,
	} {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe_test.go", src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		if got := missingTables(readableTables([]*ast.File{file})); !slices.Equal(got, []string{"shippedPackCodeRanges"}) {
			t.Errorf("%s: missing tables %v, want shippedPackCodeRanges", name, got)
		}
	}
}
