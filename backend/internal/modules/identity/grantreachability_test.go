// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Every grant a handler DEMANDS is a grant some seeded role HOLDS.
//
// `auth.Require(ctx, object, action)` is the door. The seeded role documents in
// internal/policy decide who may open it. Nothing connected the two, so a
// handler could require a verb the policy grants to nobody, and the feature
// behind it was inert on every installation with no test anywhere failing —
// the door refuses, which is exactly what a door is supposed to do, and the
// refusal is indistinguishable from a correct one.
//
// That is not hypothetical: `assurance.Store.Resolve` requires
// `forecast.update`, which no seeded role holds, including admin. The whole
// answering half of the input-check review shipped unreachable.
//
// ## Why this reads source rather than calling the handlers
//
// The alternative is an integration test per door, and the gap is precisely
// the door nobody wrote one for. A census over the tree finds the doors the
// author of the next one will not think to register.
//
// ## What it cannot see, and why that is stated rather than hidden
//
// An object computed at runtime — `auth.Require(ctx, string(spec.entity), …)`
// in the analytics query paths, an extension's own object name — is not a
// static fact and no source scan can resolve it. Those are counted rather than
// skipped, and the count is HELD: under-recognition is the one way a census
// must not break, because it reads a smaller tree, reports PASS, and leaves no
// failing assertion to notice. A new dynamic call site pushes the count past
// the recorded ceiling and fails here, which is the prompt to check that door
// by hand.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// grantedToNobody are the (object, action) pairs a handler requires that the
// seeded policy hands to no role at all. Each one is a feature that cannot be
// reached on any installation, so an entry here is an outage with a reason,
// never a style waiver.
var grantedToNobody = gatekit.Waive(map[string]string{
	// The seeded policy gives `forecast` the createRead posture on the belief
	// that "a forecast reading is derived, and a current call SUPERSEDES rather
	// than being rewritten" — true of the readings, and not true of an
	// input-check finding, which is answered in place by a named person.
	//
	// Which seats may answer one is a product call and not this gate's to make:
	// it decides whether every reader of the forecast may resolve a finding or
	// only the seats that already create one. Tracked as the open question on
	// the ticket this gate was written for. The entry stands until that lands,
	// and it is the reason the gate could be armed today rather than after it.
	"forecast.update": "no seeded role holds it, so the input-check answering path is unreachable on every installation; which seats may answer a finding is an open product decision, not a policy typo",
})

// dynamicObjectCeiling is how many `auth.Require` call sites name an object
// this scan cannot resolve — a runtime value rather than a literal or a
// package-level string constant.
//
// A ceiling rather than an exact count, because a refactor that removes one is
// not a regression and should not fail. It moves DOWN freely and up only with
// a reason: each of these is a door no static check covers, so growing the
// number is growing the blind spot this gate exists to bound.
const dynamicObjectCeiling = 106

// requireSitesFloor is the fail-short guard. The scan walking a smaller tree
// than it thinks — a moved directory, a parser error swallowed — would report
// PASS while checking nothing, so the census asserts it found roughly what the
// tree holds rather than trusting that it did.
const requireSitesFloor = 500

func TestEveryGrantAHandlerRequiresIsHeldBySomeSeededRole(t *testing.T) {
	defer grantedToNobody.AssertAllMatched(t)

	held := grantsHeldBySomeRole(t)
	required, dynamic := grantsHandlersRequire(t)

	if len(required) < requireSitesFloor {
		t.Fatalf("the scan resolved only %d auth.Require call sites, below the %d floor — "+
			"it is reading a smaller tree than this package sits in, and a census that "+
			"can fail short reports PASS while checking nothing",
			len(required), requireSitesFloor)
	}
	if dynamic > dynamicObjectCeiling {
		t.Errorf("%d auth.Require call sites name an object this scan cannot resolve, above the "+
			"ceiling of %d — each is a door no static check covers. Resolve the new one to a "+
			"literal or a package-level constant, or raise the ceiling here with the reason it "+
			"has to be computed", dynamic, dynamicObjectCeiling)
	}

	for _, site := range required {
		pair := site.object + "." + site.action
		if held[pair] || grantedToNobody.Waived(t, pair) {
			continue
		}
		t.Errorf("%s requires %q, which no seeded role holds — the feature behind this door is "+
			"unreachable on every installation, and the refusal reads exactly like a correct one. "+
			"Grant the verb in internal/policy/defaults.go (and regenerate the matrix), or record "+
			"here why nobody may pass", site.where, pair)
	}
}

// grantsHeldBySomeRole is the union across the seeded role documents: the
// question is whether ANY seat can reach the door, so a verb one role holds is
// held. Read from the same JSON the server writes into role.permissions, for
// the reason the published matrix is rendered from it — the stored document is
// what a workspace actually gets.
func grantsHeldBySomeRole(t *testing.T) map[string]bool {
	t.Helper()
	held := map[string]bool{}
	for _, role := range systemRoles {
		var doc roleDocument
		if err := json.Unmarshal(policy.MustDefaultJSON(role.key), &doc); err != nil {
			t.Fatalf("decoding the seeded document for role %q: %v", role.key, err)
		}
		for object, grant := range doc.Objects {
			for verb, granted := range map[string]bool{
				"create": grant.Create, "read": grant.Read,
				"update": grant.Update, "delete": grant.Delete,
			} {
				if granted {
					held[object+"."+verb] = true
				}
			}
		}
	}
	if len(held) == 0 {
		t.Fatal("the seeded roles grant nothing at all, so every comparison below would fail " +
			"for the wrong reason")
	}
	return held
}

// requireSite is one door, named where a reader can open it.
type requireSite struct {
	object string
	action string
	where  string
}

// grantsHandlersRequire walks the server for auth.Require calls, answering the
// resolved ones and how many named an object it could not resolve.
func grantsHandlersRequire(t *testing.T) ([]requireSite, int) {
	t.Helper()
	root := repoArtifact("internal")
	files := goSourceFiles(t, root)
	constants := packageStringConstants(t, files)

	var sites []requireSite
	dynamic := 0
	for _, path := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := requireCall(node)
			if !ok {
				return true
			}
			object, resolved := objectName(call.Args[1], constants[filepath.Dir(path)])
			if !resolved {
				dynamic++
				return true
			}
			action, ok := actionName(call.Args[2])
			if !ok {
				// The action is a parameter (`auth.Require(ctx, "person", action)`),
				// so the pair is not decided here. The object is still known and
				// every verb of it is checked at the call sites that name one.
				return true
			}
			sites = append(sites, requireSite{
				object: object, action: action,
				where: filepath.ToSlash(path) + ":" + strconv.Itoa(fset.Position(call.Pos()).Line),
			})
			return true
		})
	}
	return sites, dynamic
}

// requireCall answers whether a node is an `auth.Require(ctx, object, action)`
// call with the three arguments this scan reads.
func requireCall(node ast.Node) (*ast.CallExpr, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Require" {
		return nil, false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "auth" || len(call.Args) < 3 {
		return nil, false
	}
	return call, true
}

// objectName resolves the object argument: a string literal, or an identifier
// naming a string constant declared in the same package. Anything else is a
// runtime value and is reported unresolved rather than guessed at.
func objectName(arg ast.Expr, constants map[string]string) (string, bool) {
	if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		name, err := strconv.Unquote(lit.Value)
		return name, err == nil
	}
	if ident, ok := arg.(*ast.Ident); ok {
		value, found := constants[ident.Name]
		return value, found
	}
	return "", false
}

// actionName reads `principal.ActionCreate` and its siblings as "create".
func actionName(arg ast.Expr) (string, bool) {
	selector, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "principal" || !strings.HasPrefix(selector.Sel.Name, "Action") {
		return "", false
	}
	return strings.ToLower(strings.TrimPrefix(selector.Sel.Name, "Action")), true
}

// packageStringConstants indexes every package-level string constant by
// directory, which is what lets `auth.Require(ctx, tableProject, …)` resolve.
// Keyed by directory rather than by package name because that is the unit a
// file's unqualified identifiers actually resolve against.
func packageStringConstants(t *testing.T, files []string) map[string]map[string]string {
	t.Helper()
	byDir := map[string]map[string]string{}
	for _, path := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		dir := filepath.Dir(path)
		if byDir[dir] == nil {
			byDir[dir] = map[string]string{}
		}
		for _, decl := range file.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok || (generic.Tok != token.CONST && generic.Tok != token.VAR) {
				continue
			}
			indexStringSpecs(generic, byDir[dir])
		}
	}
	return byDir
}

// indexStringSpecs records the string-literal declarations of one const or var
// block.
func indexStringSpecs(decl *ast.GenDecl, into map[string]string) {
	for _, spec := range decl.Specs {
		values, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range values.Names {
			if i >= len(values.Values) {
				continue
			}
			lit, ok := values.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if value, err := strconv.Unquote(lit.Value); err == nil {
				into[name.Name] = value
			}
		}
	}
}

// goSourceFiles collects the non-test Go files under root, sorted so a failure
// names the same call site on every run.
//
// Completeness is not claimed here, it is HELD: a walk that quietly missed
// files would resolve too few call sites and trip requireSitesFloor above,
// which is the assertion that makes this scan unable to pass by reading less
// than the tree.
func goSourceFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}
