// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every problem code an httperr builder sends for a reader has catalog copy on
// the screen, keyed by the wire code (PROBLEM_CODE_COPY in problemcodecopy.ts).
// The detail is English, or one fixed sentence. The codes are read from the
// call sites, so no list of them is kept here.

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	httperrPath          = "github.com/margince/margince/backend/internal/platform/httperr"
	problemCopyOwner     = "../frontend/src/lib/problemcodecopy.ts"
	duplicateCopyKeyStem = "common.duplicate."
)

// codeBuilder is one httperr helper whose code a reader must get copy for,
// and which of its arguments carries that code.
type codeBuilder struct {
	name     string
	codeArg  int
	keysStem string
}

// readerCodeBuilders are the helpers this gate holds: Unavailable exists to
// carry a code for a reader, and Duplicate's detail is one sentence for every
// key. keysStem names the keys that serve only that builder, so stale copy fails.
var readerCodeBuilders = []codeBuilder{
	{name: "Unavailable", codeArg: 2},
	{name: "Duplicate", codeArg: 0, keysStem: duplicateCopyKeyStem},
}

// problemCopyBlock is the body of the code→catalog-key map.
var problemCopyBlock = regexp.MustCompile(`(?s)const PROBLEM_CODE_COPY = new Map<string, MessageKey>\(\[(.*?)\]\);`)

// problemCopyEntry is one `["code", "catalog.key"]` pair of that map.
var problemCopyEntry = regexp.MustCompile(`\[\s*"([a-z_]+)"\s*,\s*"([A-Za-z_.]+)"\s*,?\s*\]`)

func TestEveryReaderFacingProblemCodeHasClientCopy(t *testing.T) {
	t.Parallel()
	copied := problemCodeCopy(t)
	for _, builder := range readerCodeBuilders {
		sent := sentProblemCodes(t, builder)
		for _, code := range sortedKeys(sent) {
			if _, ok := copied[code]; !ok {
				t.Errorf("%s sends httperr.%s code %q and %s maps no copy to it, so the reader gets the "+
					"server's English detail whatever language they set; add the entry and its en, de and vi messages",
					sent[code], builder.name, code, problemCopyOwner)
			}
		}
		if builder.keysStem == "" {
			continue
		}
		for _, code := range sortedKeys(copied) {
			if _, ok := sent[code]; !ok && strings.HasPrefix(copied[code], builder.keysStem) {
				t.Errorf("%s maps %q to %s, and no httperr.%s call sends that code; remove the entry and its messages",
					problemCopyOwner, code, copied[code], builder.name)
			}
		}
	}
}

// sentProblemCodes reads the code of every call to the builder in the module,
// keyed by code, valued by one file that sends it.
func sentProblemCodes(t *testing.T, builder codeBuilder) map[string]string {
	t.Helper()
	files := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return gatekit.References(file, httperrPath, builder.name)
		},
	}.Files(t)
	sent := map[string]string{}
	consts := map[string]map[string]string{}
	for _, parsed := range files {
		dir := path.Dir(parsed.Path)
		if consts[dir] == nil {
			consts[dir] = gatekit.PackageStringConstants(t, filepath.FromSlash(dir))
		}
		inPackage := parsed.File.Name.Name == path.Base(httperrPath)
		codes, faults := builderCodes(gatekit.SourceFileSet(), parsed.File, builder, inPackage, consts[dir])
		for _, fault := range faults {
			t.Errorf("%s: %s", parsed.Path, fault)
		}
		for _, code := range codes {
			sent[code] = parsed.Path
		}
	}
	// Each builder answers real requests today, so an empty read means the
	// walk went blind rather than that the tree is clean.
	if len(sent) == 0 {
		t.Fatalf("read no httperr.%s codes under internal/: the walk has gone blind", builder.name)
	}
	return sent
}

// builderCodes reads the code argument of each call to the builder in file.
// A code it cannot fold to a string is a fault. So is the builder passed as
// a value. Either way a code would escape the census.
func builderCodes(fset *token.FileSet, file *ast.File, builder codeBuilder, inPackage bool,
	consts map[string]string,
) (codes, faults []string) {
	qualifier, dotImported := gatekit.ImportedAs(file, httperrPath)
	if dotImported {
		return nil, []string{"imports httperr with a dot, so its calls cannot be told from local ones; import it by name"}
	}
	if qualifier == "" && !inPackage {
		return nil, nil
	}
	packageLevel := packageLevelSpecs(file)
	declared := map[ast.Node]bool{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			declared[fn.Name] = true
		}
	}
	names := func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && qualifier != "" {
			pkg, isIdent := selector.X.(*ast.Ident)
			return isIdent && pkg.Name == qualifier && selector.Sel.Name == builder.name
		}
		ident, ok := node.(*ast.Ident)
		return ok && inPackage && ident.Name == builder.name && !declared[ident]
	}
	line := func(n ast.Node) int { return fset.Position(n.Pos()).Line }
	called := map[ast.Node]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && names(call.Fun) {
			called[call.Fun] = true
			code, readable := "", false
			if len(call.Args) > builder.codeArg && !namesLocal(call.Args[builder.codeArg], packageLevel) {
				code, readable = gatekit.StringExpr(call.Args[builder.codeArg], consts, gatekit.FoldStrict)
			}
			if !readable {
				faults = append(faults, fmt.Sprintf("line %d: the code passed to httperr.%s cannot be read; "+
					"pass a literal or a package constant", line(call), builder.name))
				return true
			}
			codes = append(codes, code)
			return true
		}
		if names(node) && !called[node] {
			faults = append(faults, fmt.Sprintf("line %d: httperr.%s is used as a value, so its code cannot "+
				"be read; call it directly", line(node), builder.name))
		}
		return true
	})
	return codes, faults
}

// packageLevelSpecs lists the file's top-level value declarations. A name the
// parser resolved to anything else is local, and may shadow a package constant.
func packageLevelSpecs(file *ast.File) map[any]bool {
	specs := map[any]bool{}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				specs[spec] = true
			}
		}
	}
	return specs
}

// namesLocal reports whether expr names a parameter or local, which the
// package-constant fold would otherwise read by its name alone. A name
// declared in a sibling file is left unresolved, and is package-level.
func namesLocal(expr ast.Expr, packageLevel map[any]bool) bool {
	local := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && ident.Obj != nil && !packageLevel[ident.Obj.Decl] {
			local = true
		}
		return !local
	})
	return local
}

// problemCodeCopy reads the screen's code→catalog-key map.
func problemCodeCopy(t *testing.T) map[string]string {
	t.Helper()
	source, err := os.ReadFile(problemCopyOwner)
	if err != nil {
		t.Fatalf("reading %s: %v", problemCopyOwner, err)
	}
	copied, err := parseProblemCodeCopy(string(source))
	if err != nil {
		t.Fatalf("%s: %v", problemCopyOwner, err)
	}
	return copied
}

// parseProblemCodeCopy reads PROBLEM_CODE_COPY with comments removed, so a line
// that only mentions an entry cannot stand in for one. Every tuple bracket must
// be a pair the entry pattern read, or a stale entry would escape the check.
func parseProblemCodeCopy(source string) (map[string]string, error) {
	block := problemCopyBlock.FindStringSubmatch(source)
	if block == nil {
		return nil, errors.New("no PROBLEM_CODE_COPY map in the shape this gate reads")
	}
	body := tsComment.ReplaceAllString(block[1], " ")
	entries := problemCopyEntry.FindAllStringSubmatch(body, -1)
	if len(entries) == 0 {
		return nil, errors.New("read no entries from PROBLEM_CODE_COPY: the walk has gone blind")
	}
	if tuples := strings.Count(tsString.ReplaceAllString(body, `""`), "["); tuples != len(entries) {
		return nil, fmt.Errorf("PROBLEM_CODE_COPY holds %d entries and %d match the [\"code\", \"catalog.key\"] "+
			"shape this gate reads; write every entry in that shape", tuples, len(entries))
	}
	copied := map[string]string{}
	for _, entry := range entries {
		if _, seen := copied[entry[1]]; seen {
			return nil, fmt.Errorf("PROBLEM_CODE_COPY maps %q twice; keep one entry", entry[1])
		}
		copied[entry[1]] = entry[2]
	}
	return copied, nil
}

// An entry outside the pattern, or a repeated code, fails the read instead of
// dropping out of the "no call sends this code" check.
func TestProblemCodeCopyRefusesAnEntryItCannotRead(t *testing.T) {
	t.Parallel()
	const wrap = "const PROBLEM_CODE_COPY = new Map<string, MessageKey>([\n%s\n]);"
	for name, body := range map[string]string{
		"digit in code":    `["duplicate_email", "common.duplicate.email"], ["duplicate_email2", "common.duplicate.email"],`,
		"single quotes":    `["duplicate_email", "common.duplicate.email"], ['duplicate_domain', 'common.duplicate.domain'],`,
		"repeated code":    `["duplicate_email", "common.duplicate.email"], ["duplicate_email", "common.duplicate.domain"],`,
		"constant as code": `["duplicate_email", "common.duplicate.email"], [CODE, "common.duplicate.domain"],`,
	} {
		if copied, err := parseProblemCodeCopy(fmt.Sprintf(wrap, body)); err == nil {
			t.Errorf("%s: read %v, want a refusal", name, copied)
		}
	}
	copied, err := parseProblemCodeCopy(fmt.Sprintf(wrap, `// ["gone", "common.gone"],
  ["duplicate_email", "common.duplicate.email"],`))
	if err != nil || len(copied) != 1 {
		t.Errorf("a well-formed map read %v, %v; want its one entry", copied, err)
	}
}

// The walk is planted with every call shape it claims to read, and with the
// two it must refuse. A change that narrows it fails here instead.
func TestProblemCodeWalkReadsEveryPlantedShape(t *testing.T) {
	t.Parallel()
	const planted = `package planted

import he "github.com/margince/margince/backend/internal/platform/httperr"

const codeDomain = "duplicate_domain"

func literal() error  { return he.Duplicate("duplicate_email", "") }
func constant() error { return he.Duplicate(codeDomain, "id") }
func computed(code string) error { return he.Duplicate(code, "") }
func asValue() func(string, string) *he.DetailedError { return he.Duplicate }
func other(w http.ResponseWriter, r *http.Request) { he.Unavailable(w, r, "gateway_down", "") }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "planted.go", planted, 0)
	if err != nil {
		t.Fatalf("parsing the planted package: %v", err)
	}
	// `code` is also a package constant here, so folding by name alone would
	// read the parameter in computed() as a code.
	consts := map[string]string{"codeDomain": "duplicate_domain", "code": "lead_holds_email"}
	for _, probe := range []struct {
		builder codeBuilder
		codes   string
		faults  int
	}{
		{builder: readerCodeBuilders[1], codes: "duplicate_email,duplicate_domain", faults: 2},
		{builder: readerCodeBuilders[0], codes: "gateway_down", faults: 0},
	} {
		codes, faults := builderCodes(fset, file, probe.builder, false, consts)
		if got := strings.Join(codes, ","); got != probe.codes {
			t.Errorf("httperr.%s: read codes %q, want %q", probe.builder.name, got, probe.codes)
		}
		if len(faults) != probe.faults {
			t.Errorf("httperr.%s: faulted %d planted shapes, want %d: %q", probe.builder.name, len(faults), probe.faults, faults)
		}
	}
}

// A dot import hides which bare calls are httperr's, so it is refused whole.
func TestProblemCodeWalkRefusesADotImport(t *testing.T) {
	t.Parallel()
	const planted = `package planted

import . "github.com/margince/margince/backend/internal/platform/httperr"

func literal() error { return Duplicate("new_code", "") }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "planted.go", planted, 0)
	if err != nil {
		t.Fatalf("parsing the planted package: %v", err)
	}
	codes, faults := builderCodes(fset, file, readerCodeBuilders[1], false, nil)
	if len(codes) != 0 || len(faults) != 1 {
		t.Errorf("read codes %q and faults %q, want no codes and one fault", codes, faults)
	}
}
