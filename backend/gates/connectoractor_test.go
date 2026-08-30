// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A connector's actor id is DERIVED from the work, never written down.
//
// Every value a provider run writes is bought from a vendor rather than typed
// by anybody, so the audit row names the connector — and WHICH connector is a
// fact about the run. It was written down instead: `connector:surfe`, bound by
// the workers that execute a run, correct only while provider_connection,
// provider_run and person_provider_claim each carried a CHECK pinning them to
// one provider. Those checks are gone so a second vendor can be connected.
//
// A LITERAL IS NOT A BUG UNTIL THERE ARE TWO, which is exactly why it needs a
// gate: nothing failed while one vendor was the only one, the claim rows had
// already been made to derive their provenance from the run's own provider, and
// the disagreement appears for the first time on the first installation to
// connect a second. An audit entry naming the wrong actor is worse than a
// missing one, because it reads as authoritative.
//
// So: no principal anywhere in the tree may take a `connector:` id from a
// string literal. Assembling one from a value is how a caller says it read
// which vendor this is.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// connectorActorPrefix is what a connector's actor id starts with, matching the
// provenance written onto the rows the same act produces.
const connectorActorPrefix = "connector:"

// oneOfItsKindForever names the written-down connector ids that are not a
// vendor at all, and so have nothing to derive from.
//
// The rule is about work whose vendor is chosen at RUN TIME. A subsystem that
// records itself as a connector because its values are not typed by anybody is
// the same actor on every installation and every pass, and assembling that name
// from somewhere would be indirection with no second case behind it.
//
// What would make an entry here wrong is the thing that made connector:surfe
// wrong: the name becoming a choice. AssertAllMatched keeps the list honest
// about existing; a reader who finds one of these selecting among vendors
// should delete the entry rather than widen it.
var oneOfItsKindForever = gatekit.Waive(map[string]string{
	"connector:finance": "the finance mirror's own name, not a vendor's: one ledger sweep per installation, converting the source ledger into the base currency as it writes. There is nothing to read the name from, because there is never more than one",
})

// TestNoPrincipalNamesAConnectorInAStringLiteral is the census.
func TestNoPrincipalNamesAConnectorInAStringLiteral(t *testing.T) {
	t.Parallel()
	read := 0
	err := filepath.WalkDir("internal", func(walked string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(walked, ".go") ||
			strings.HasSuffix(walked, "_test.go") || isIntegrationTagged(walked) {
			return err
		}
		walked = filepath.ToSlash(walked)
		file, err := parser.ParseFile(token.NewFileSet(), walked, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, named := range principalIDsWrittenIn(file) {
			read++
			if !strings.HasPrefix(named, connectorActorPrefix) || oneOfItsKindForever.Waived(t, named) {
				continue
			}
			t.Errorf("%s: builds a principal whose id is the literal %q, so every run this actor is bound for is "+
				"audited as that vendor whichever one it is actually for. Read the provider from the work and "+
				"assemble the id from it — the rows the same act writes already derive their provenance that way",
				walked, named)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	oneOfItsKindForever.AssertAllMatched(t)
	// A tree where no principal carries a written-down id at all would read
	// exactly like a clean one. There are several, all of them named after a
	// job or a subsystem rather than a vendor.
	if read == 0 {
		t.Fatal("this census found no principal built with a literal id, and there are several in this tree: " +
			"the reader has stopped matching rather than the tree having changed, and it cannot tell the difference")
	}
}

// principalIDsWrittenIn names the string literals a file assigns to a
// principal's ID field.
//
// It reads the FIELD, not the file's strings: `connector:` appears in
// provenance writers, in prefix tests and in SQL, and none of those binds an
// actor. What this is about is who a piece of work is recorded as.
//
// The qualifier comes from the file's own import block, so a file that aliases
// the principal package is read as one that does not.
func principalIDsWrittenIn(file *ast.File) []string {
	qualifier, dotImported := gatekit.ImportedAs(file, principalImportPath)
	if qualifier == "" && !dotImported {
		return nil
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !namesAPrincipal(lit.Type, qualifier, dotImported) {
			return true
		}
		for _, element := range lit.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			if !isField {
				continue
			}
			if key, isIdent := field.Key.(*ast.Ident); !isIdent || key.Name != "ID" {
				continue
			}
			value, isBasic := field.Value.(*ast.BasicLit)
			if !isBasic || value.Kind != token.STRING {
				// Anything assembled — a concatenation, a call, a variable —
				// is a value the caller read from somewhere, which is the
				// shape this gate is asking for.
				continue
			}
			if unquoted, err := strconv.Unquote(value.Value); err == nil {
				out = append(out, unquoted)
			}
		}
		return true
	})
	return out
}

// namesAPrincipal reports whether a composite literal builds a principal,
// including one taken by address.
func namesAPrincipal(expr ast.Expr, qualifier string, dotImported bool) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		return namesAPrincipal(star.X, qualifier, dotImported)
	}
	return isPrincipalType(expr, qualifier, dotImported)
}
