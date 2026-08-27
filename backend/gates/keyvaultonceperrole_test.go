// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A role resolves its key vault ONCE.
//
// cmd/worker used to resolve it three times — the Surface-B runner lane, the
// connector-credential backfill, the job runner — and each read the answer in
// its own way: two nil-ed the vault on the `configured` flag, one passed
// through whatever FromEnv returned. cmd/api had the same shape in three
// phases. Nothing diverged in practice, because the flag and the nil say the
// same thing, but "does this deployment have a vault?" had three answers in one
// process and only a coincidence kept them equal. That is the kind of
// divergence an operator reports as unreproducible.
//
// So the roles cannot ask. keyvault.FromEnv is unreachable from cmd/, and
// keyvault.ForRole — which resolves once, hands back a vault or nil, and lets
// no flag out — may be called once per role. A fourth lane needing a vault now
// takes the one its boot already holds.
//
// The FromEnv census is module-wide rather than cmd-only, because a gate that
// judges one subtree also claims that the subtree is where the obligated code
// lives, and that second claim is the one nothing checks: boot wiring that
// moved into internal/ would walk straight out of reach with the gate still
// green. Detection is over the syntax tree rather than the text, through
// gatekit.References, so an aliased import is caught the same as a plain one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// keyvaultPath is the vault's package, matched by import path rather than by
// the identifier a file happens to bind it to.
const keyvaultPath = "github.com/margince/margince/backend/internal/platform/keyvault"

// ownResolvers are the non-test files outside the keyvault package ratified to
// resolve a vault of their own, each bound to why a boot-owned one will not do.
var ownResolvers = gatekit.Waive(map[string]string{
	"internal/compose/routingsource.go": "resolves per WORKSPACE and per routing change, not per boot: it " +
		"upgrades a caller-supplied env lookup — not necessarily the process environment — into one that " +
		"unseals that workspace's BYOK keys, and the routing watcher re-runs it while the role serves. It " +
		"also treats an unbuildable vault as a warning rather than a boot refusal, deliberately, because " +
		"the serving roles have already had their say on that by the time it runs. A boot vault threaded " +
		"in would answer a different question under the same name.",
})

func TestTheKeyVaultIsResolvedOncePerRole(t *testing.T) {
	t.Parallel()
	var resolvers, roleCalls []string
	callsPerRole := map[string]int{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		p = filepath.ToSlash(p)
		// The vault's own package is where FromEnv is SUPPOSED to be reached:
		// ForRole is built on it. Excluded by rule rather than waived, because a
		// waiver would read as "this one is allowed to misbehave" when it is the
		// definition of behaving.
		if strings.HasPrefix(p, keyvaultDir+"/") {
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		if gatekit.References(file, keyvaultPath, "FromEnv") && !ownResolvers.Waived(t, p) {
			resolvers = append(resolvers, p)
		}
		if n := countsForRole(file); n > 0 && strings.HasPrefix(p, "cmd/") {
			role := path.Dir(p)
			callsPerRole[role] += n
			roleCalls = append(roleCalls, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}

	for _, p := range resolvers {
		if strings.HasPrefix(p, "cmd/") {
			t.Errorf("%s resolves its own key vault. A role's boot resolves ONE, at a single ownership "+
				"boundary, and hands it to every lane that needs it — take the vault your boot already "+
				"holds, or call keyvault.ForRole where that boot is.", p)
			continue
		}
		t.Errorf("%s calls keyvault.FromEnv and is not ratified to. Boot code takes the vault its role "+
			"resolved (keyvault.ForRole); a path that genuinely needs its own — a different lookup, a "+
			"different lifetime, a different verdict on failure — says which, in ownResolvers.", p)
	}

	for role, n := range callsPerRole {
		if n > 1 {
			t.Errorf("%s calls keyvault.ForRole %d times. ForRole is the role's ONE resolution; a second "+
				"is a second answer to a question that has one.", role, n)
		}
	}

	// Both halves must stay live. If nothing calls ForRole any more the count
	// check passes while checking nothing, and if the walk stops finding Go
	// files the census reports PASS over an empty tree.
	if len(roleCalls) == 0 {
		t.Error("no role calls keyvault.ForRole — either the roles stopped resolving a vault at boot, " +
			"or this gate has gone blind and its count check now passes vacuously")
	}
	ownResolvers.AssertAllMatched(t)
}

// keyvaultDir is the vault package's directory, derived from the import path
// above rather than written out a second time.
var keyvaultDir = strings.TrimPrefix(keyvaultPath, "github.com/margince/margince/backend/")

// countsForRole counts the CALL SITES of keyvault.ForRole in one file. A count
// rather than gatekit.References's boolean, because the thing being prohibited
// is a second resolution and one file can hold both.
//
// Syntactic call sites, which is what a static gate can honestly say: a single
// call inside a function the boot invokes twice is out of reach here, and so is
// one behind a stored func value. Neither is a shape this boot code has, and a
// gate that pretended otherwise would be claiming a guarantee it does not hold.
func countsForRole(file *ast.File) int {
	qualifier, dotImported := gatekit.ImportedAs(file, keyvaultPath)
	if qualifier == "" && !dotImported {
		return 0
	}
	calls := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			pkg, ok := fn.X.(*ast.Ident)
			if ok && pkg.Name == qualifier && qualifier != "" && fn.Sel.Name == "ForRole" {
				calls++
			}
		case *ast.Ident:
			if dotImported && fn.Name == "ForRole" {
				calls++
			}
		}
		return true
	})
	return calls
}
