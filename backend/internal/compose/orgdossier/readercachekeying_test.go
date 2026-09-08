// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgdossier

// A reader cache is keyed on the READER as well as the company.
//
// These caches hold one reader's assembled view of a company — assembled from
// what that reader may see, so two readers of the same company hold legitimately
// different rows. A statement keyed on the organization alone breaks that two
// ways at once and neither is loud: a read serves somebody else's assessment,
// and a write evicts it.
//
// The end-to-end proof that it behaves this way exists for growth-fit
// (TestTheGrowthFitCacheRowIsKeyedToTheReaderWhoAskedForIt, compose/integration).
// It costs a booted app and a real read, which is why there is one of it and not
// one per cache — and why the dossier cache, the same type with the same shape,
// had no proof at all. This is the cheap half that covers every cache: the
// statements themselves, read off the values the package actually uses.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// readerCacheStatements collects the readerCache values this package declares,
// by variable name, with the two statements each one holds.
//
// From the SOURCE rather than from a list written here: a third cache added
// beside these two is judged without anybody remembering this test, which is
// the case that matters — the dossier cache went unproven precisely because the
// growth-fit one had a test and nothing asked about its sibling.
func readerCacheStatements(t *testing.T) map[string]map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "cache.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing cache.go: %v", err)
	}
	caches := map[string]map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		if named, ok := lit.Type.(*ast.Ident); !ok || named.Name != "readerCache" {
			return true
		}
		statements := map[string]string{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			field, isField := kv.Key.(*ast.Ident)
			// The string, not the source text. A raw literal's own text carries
			// its backticks, so every substring check below would be asking
			// about a string Postgres never receives — the difference is
			// invisible until an escape or a quote style changes and the census
			// quietly stops matching.
			sql, isLiteral := gatekit.StringExpr(kv.Value, nil, gatekit.FoldStrict)
			if isField && isLiteral {
				statements[field.Name] = sql
			}
		}
		caches[spec.Names[0].Name] = statements
		return true
	})
	return caches
}

func TestEveryReaderCacheIsKeyedOnTheReaderAndNotTheCompanyAlone(t *testing.T) {
	caches := readerCacheStatements(t)
	if len(caches) == 0 {
		t.Fatal("cache.go declares no readerCache value — this test read an empty corpus, and would " +
			"report every cache correctly keyed without looking at one")
	}
	for name, statements := range caches {
		for _, field := range []string{"selectOne", "upsertOne"} {
			if _, present := statements[field]; !present {
				t.Errorf("%s declares no %s, so half of its keying is unstated and unchecked", name, field)
			}
		}
		// The READ, or one reader is served another's assessment of a company
		// they see differently.
		if !strings.Contains(statements["selectOne"], "user_id = $1") {
			t.Errorf("%s.selectOne does not bind user_id, so it serves whichever reader's row it finds "+
				"first — an assessment assembled from what somebody else could see", name)
		}
		// The WRITE, and this is the half the end-to-end test could not see
		// until #665: a conflict target without user_id makes one row per
		// company, so a second reader's read evicts the first's.
		if !strings.Contains(statements["upsertOne"], "ON CONFLICT (user_id, organization_id)") {
			t.Errorf("%s.upsertOne does not resolve conflicts on (user_id, organization_id), so the cache "+
				"holds one row per company rather than one per reader and each read evicts the last "+
				"reader's row", name)
		}
	}
}
