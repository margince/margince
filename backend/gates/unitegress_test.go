// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A unit dials through the installation's egress policy, not through a copy of
// it.
//
// The policy is published — extension.ReservedNets, extension.RefuseNonPublic,
// extension.OutboundClient — precisely so a unit that fetches a
// member-supplied URL does not have to write one. What this refuses is the
// alternative: a unit spelling the reserved ranges itself, or wiring its own
// net.Dialer.Control.
//
// The hazard is not that a copy is written badly. The first one was written
// well — the same eleven ranges, the same post-resolution hook, reviewed side
// by side. The hazard is that it stops being equal the moment the core list
// moves, silently, in a module nothing in this tree compiles against the core.
// A range added because somebody found a way to name an internal address is
// then absent from every unit that copied it, and the failure is a worker
// fetching an address the installation refuses everywhere else.
//
// WHAT IT CANNOT SEE, said plainly: a unit that builds the denylist from
// computed strings, or wires a Control hook through a variable this cannot
// resolve. Both defeat a static reader by construction. This is a lint against
// the shape a unit actually writes — the same posture
// TestUnitSQLAddressesItsOwnTables takes about a unit's SQL, and for the same
// reason: the wall is elsewhere and this catches the mistake.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// unitTrees are where a unit's Go lives. fixtures included: a reference unit is
// what an author copies, so a copy of the policy there is the one most likely
// to be reproduced.
var unitTrees = []string{"../extensions", "../fixtures/extensions"}

// reservedCIDRLiteral matches a CIDR string. It is the SHAPE rather than the
// eleven values, because a unit hand-copying the list is as likely to have
// typed a range the core does not carry — which is a copy either way, and the
// one that admits an address the core refuses is the worse half.
var reservedCIDRLiteral = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$|^[0-9a-fA-F:]+::?[0-9a-fA-F:]*/\d{1,3}$`)

// publishedEgress are the names on the published surface a unit reaches the
// policy through. A dialer whose Control is one of these is doing what the
// surface documents — composing its own client around the published hook — and
// is not reported.
var publishedEgress = []string{"RefuseNonPublic", "OutboundClient", "OutboundTransport", "ReservedNets"}

// extensionSurface is the import path those names have to come from. Matching
// the SPELLING alone would accept a unit that declared its own
// RefuseNonPublic and wired that — which is the copy this gate exists to
// refuse, under the one name guaranteed not to be reported.
const extensionSurface = "github.com/margince/margince/backend/pkg/extension"

func TestNoUnitDialsAroundTheInstallationsEgressPolicy(t *testing.T) {
	t.Parallel()
	var offences []string
	scanned := 0
	for _, tree := range unitTrees {
		err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			scanned++
			offences = append(offences, unitEgressOffences(path, file)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s for a unit's own egress policy: %v", tree, err)
		}
	}
	// A prohibition over an empty corpus prohibits nothing, and this one's
	// corpus is other modules' source: a moved tree, a renamed fixtures
	// directory, or a walk that stopped finding .go files all read exactly
	// like a clean set of units.
	if scanned < 20 {
		t.Fatalf("read %d unit source file(s) across %v, and the tree ships more than that: this gate is "+
			"looking somewhere the units are not", scanned, unitTrees)
	}
	for _, offence := range offences {
		t.Error(offence)
	}
}

// unitEgressOffences names each place one unit file writes its own egress
// policy.
func unitEgressOffences(path string, file *ast.File) []string {
	// The file's own alias for the published surface, so a hook is recognised
	// by WHERE it comes from rather than by what it is called.
	surface, dotImported := gatekit.ImportedAs(file, extensionSurface)
	if dotImported {
		// A dot-import puts the published names in the file's own scope, where
		// this cannot tell them from a local declaration of the same name. No
		// unit does it, and the honest answer is to say so rather than to
		// guess: reported, so somebody looks.
		return []string{path + ": dot-imports the published surface, and this gate cannot then tell " +
			"extension.RefuseNonPublic from a local function of that name — import it qualified"}
	}
	var offences []string
	ast.Inspect(file, func(node ast.Node) bool {
		if literal, isLiteral := node.(*ast.BasicLit); isLiteral && literal.Kind == token.STRING {
			text, err := strconv.Unquote(literal.Value)
			if err == nil && reservedCIDRLiteral.MatchString(text) {
				offences = append(offences, path+": spells the network "+text+" itself — the installation's "+
					"denylist is published as extension.ReservedNets, and a copy stops being equal to it "+
					"the moment the core list moves, in a module nothing here compiles against the core")
			}
			return true
		}
		if dialerControl(node, surface) {
			offences = append(offences, path+": wires its own net.Dialer.Control — the guarded dialer is "+
				"published as extension.OutboundClient (and extension.RefuseNonPublic for a unit that "+
				"needs its own client settings), so a hand-written hook is a second answer to one question")
		}
		return true
	})
	return offences
}

// dialerControl reports a net.Dialer composite literal that sets Control to
// anything other than the published hook.
func dialerControl(node ast.Node, surface string) bool {
	lit, isLit := node.(*ast.CompositeLit)
	if !isLit || !isNetDialer(lit.Type) {
		return false
	}
	for _, element := range lit.Elts {
		kv, isKV := element.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if !isIdent || key.Name != "Control" {
			continue
		}
		return !namesPublishedEgress(kv.Value, surface)
	}
	return false
}

func isNetDialer(expr ast.Expr) bool {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Dialer" {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && pkg.Name == "net"
}

// namesPublishedEgress reports whether an expression reaches one of the
// published names THROUGH THE PUBLISHED PACKAGE, so a unit wrapping
// extension.RefuseNonPublic in its own dialer is not reported for doing what
// the surface tells it to.
//
// Qualified, never bare. A unit that declared its own func RefuseNonPublic and
// wired that would otherwise pass under the one name this gate is guaranteed
// not to report — the copy, wearing the name of the thing it copies. surface is
// the file's own alias for the published package, and is empty when the file
// does not import it at all, in which case nothing here can be reached and
// every hook is the unit's own.
func namesPublishedEgress(expr ast.Expr, surface string) bool {
	if surface == "" {
		return false
	}
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		pkg, isIdent := selector.X.(*ast.Ident)
		if !isIdent || pkg.Name != surface {
			return true
		}
		for _, name := range publishedEgress {
			if selector.Sel.Name == name {
				found = true
			}
		}
		return !found
	})
	return found
}
