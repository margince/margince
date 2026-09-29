// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every object the maskable-field catalog offers is an object some module
// actually withholds on.
//
// The catalog is one file and the enforcement is per module, and a module may
// not import a sibling: deals/maskablefields_test.go renders the deal's pairs,
// contacts/partnermaskablefields_test.go the partner's, and each reads only its
// own object's lines. Between them sits a hole neither can see — an object
// listed in the catalog that no module masks at all. The database accepts a
// mask naming it, an administrator reads the configuration back unchanged, and
// nothing anywhere is withheld.
//
// ONE DIRECTION ONLY, and that is a decision rather than a half-job. A masked
// object the catalog does not offer is the ledger entry: `commission` is
// withheld as a consequence of the partner's mask, through auth's group
// closure, and offering it a second time would let an operator configure the
// consequence, believe the tier is hidden, and leave the partner reading it.
//
// The masked objects are DERIVED from the call sites, so a module that starts
// masking a new object widens this census with no second edit.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// fieldMaskPackage declares the pass every masked record type goes through.
	fieldMaskPackage = modulePath + "/internal/platform/auth"
	applyFieldMasks  = "ApplyFieldMasks"
	// maskObjectArg is where the RBAC object sits in the call.
	maskObjectArg = 2
)

// maskedObjectScope claims that everything masking a record lives in the module
// tier, and proves it by reporting a call site anywhere else.
var maskedObjectScope = gatekit.Scope{
	Roots:   []string{"internal/modules"},
	Subject: func(_ string, file *ast.File) bool { return len(maskObjectArgs(file)) > 0 },
}

func TestEveryOfferedMaskObjectIsOneSomeModuleWithholdsOn(t *testing.T) {
	t.Parallel()

	masked := maskedObjects(t)
	offered := catalogObjects(t)
	if len(offered) == 0 {
		t.Fatalf("%s offers no object at all, so this census would agree with a tree that masked "+
			"nothing just as happily", maskableCatalog)
	}
	for _, object := range offered {
		if masked[object] != "" {
			continue
		}
		t.Errorf("%s offers masks on %q and no module withholds a field on it: a mask naming it is "+
			"stored, loaded onto every principal holding the role, and withholds nothing. Masked "+
			"objects found: %s", maskableCatalog, object, strings.Join(sortedKeysOf(masked), ", "))
	}
}

// maskedObjects collects the RBAC objects the module tier points the mask pass
// at, mapped to a file that does so, so a failure says where to look.
//
// The object is resolved through the tier's package constants because that is
// how every call spells it, and the constant routinely lives in another file of
// the same package than the call that passes it.
func maskedObjects(t *testing.T) map[string]string {
	t.Helper()
	consts := stringConstsByPackage(t, token.NewFileSet(), maskedObjectScope.Roots)
	found := make(map[string]string)
	for _, source := range maskedObjectScope.Files(t) {
		dir := filepath.ToSlash(filepath.Dir(source.Path))
		for _, arg := range maskObjectArgs(source.File) {
			if object := resolvedObject(t, arg, consts[dir], dir, source.Path); object != "" {
				found[object] = source.Path
			}
		}
	}
	return found
}

// maskObjectArgs is the object argument of every mask-pass call in one file.
// The qualifier is the importing file's own choice, so it is read from the
// imports rather than assumed.
func maskObjectArgs(file *ast.File) []ast.Expr {
	qualifier, dotImported := gatekit.ImportedAs(file, fieldMaskPackage)
	if qualifier == "" && !dotImported {
		return nil
	}
	var args []ast.Expr
	ast.Inspect(file, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if isCall && len(call.Args) > maskObjectArg && namesTheMaskPass(call.Fun, qualifier, dotImported) {
			args = append(args, call.Args[maskObjectArg])
		}
		return true
	})
	return args
}

func namesTheMaskPass(fun ast.Expr, qualifier string, dotImported bool) bool {
	switch named := fun.(type) {
	case *ast.SelectorExpr:
		pkg, isIdent := named.X.(*ast.Ident)
		return isIdent && qualifier != "" && pkg.Name == qualifier && named.Sel.Name == applyFieldMasks
	case *ast.Ident:
		return dotImported && named.Name == applyFieldMasks
	}
	return false
}

// resolvedObject reads the object out of the argument. An argument this cannot
// read is REPORTED rather than skipped: a census that quietly drops a call site
// it did not understand reads a smaller tree and reports the clean result it
// never obtained.
func resolvedObject(t *testing.T, arg ast.Expr, consts map[string]string, dir, path string) string {
	switch named := arg.(type) {
	case *ast.BasicLit:
		if value, isString := stringConst(named); isString {
			return value
		}
	case *ast.Ident:
		if value, known := consts[named.Name]; known {
			return value
		}
		if value, known := borrowedConst(t, named.Name, dir); known {
			return value
		}
	}
	t.Errorf("%s calls the mask pass with an object this census cannot read, so the catalog is "+
		"checked against a smaller set of masked objects than the tree holds: name it with a "+
		"package-level string constant or a literal", path)
	return ""
}

// borrowedConst reads a package constant declared as ANOTHER package's
// constant — `const maskObject = fieldmask.Deal`, which is how a module names
// an object whose vocabulary sits in tier 0 where every surface can reach it.
// Folding only a literal would leave this census blind to a call site the
// moment the object stopped being spelled twice.
func borrowedConst(t *testing.T, name, dir string) (string, bool) {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s for %s: %v", dir, name, err)
	}
	for _, source := range sources {
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		if selector, declared := constDeclaredAs(file, name); declared {
			return constFromPackage(t, file, selector)
		}
	}
	return "", false
}

// constDeclaredAs finds a package-level constant bound to a qualified name.
func constDeclaredAs(file *ast.File, name string) (*ast.SelectorExpr, bool) {
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || len(value.Values) != 1 || value.Names[0].Name != name {
				continue
			}
			if selector, qualified := value.Values[0].(*ast.SelectorExpr); qualified {
				return selector, true
			}
		}
	}
	return nil, false
}

// constFromPackage reads the named constant out of the package the file
// imports under that qualifier. A path outside this module is not resolvable
// from source, and the caller reports it like any other unreadable argument.
func constFromPackage(t *testing.T, file *ast.File, selector *ast.SelectorExpr) (string, bool) {
	t.Helper()
	qualifier, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if named, _ := gatekit.ImportedAs(file, imported); named != qualifier.Name {
			continue
		}
		local, inside := strings.CutPrefix(imported, modulePath+"/")
		if !inside {
			return "", false
		}
		value, known := gatekit.PackageStringConstants(t, local)[selector.Sel.Name]
		return value, known
	}
	return "", false
}

// catalogObjects is the distinct objects the catalog offers masks on.
func catalogObjects(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(maskableCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", maskableCatalog, err)
	}
	seen := make(map[string]bool)
	// catalogPair, not a second split: one parser decides what a catalog line
	// is, and a line this dropped where that one refuses would leave the
	// census sweeping fewer objects with nothing to say so.
	for number, line := range strings.Split(string(body), "\n") {
		object, _, err := catalogPair(line)
		if err != nil {
			t.Fatalf("%s line %d: %v", maskableCatalog, number+1, err)
		}
		if object != "" {
			seen[object] = true
		}
	}
	return sortedKeysOf(seen)
}
