// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"cmp"
	"go/ast"
	"go/types"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func historyEvidenceFields(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, kind := range []string{"company", "contact"} {
		pattern := `(?m)^public.field_provenance.field_provenance_` + kind + `_profile_field CHECK .*ARRAY\[([^\n]+)\]`
		match := regexp.MustCompile(pattern).FindSubmatch(raw)
		if match == nil {
			t.Fatalf("no finite %s evidence field constraint", kind)
		}
		for _, field := range regexp.MustCompile(`'([^']+)'::text`).FindAllSubmatch(match[1], -1) {
			out[kind] = append(out[kind], string(field[1]))
		}
	}
	start := strings.Index(string(raw), "public.company_fact_category(field text)")
	if start < 0 {
		t.Fatal("no company fact vocabulary")
	}
	body, _, _ := strings.Cut(string(raw)[start:], "\n\n")
	for _, arm := range regexp.MustCompile(`WHEN field IN \(([^)]+)\)`).FindAllStringSubmatch(body, -1) {
		for _, field := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(arm[1], -1) {
			out["company"] = append(out["company"], field[1])
		}
	}
	return out
}

// The merge reads reach declarations, not record columns chosen by a caller.
func historyReachEntities(t *testing.T) []string {
	t.Helper()
	var entities []string
	for _, owned := range corereach.Tables() {
		entities = append(entities, cmp.Or(owned.Table.Entity, owned.Table.Name))
	}
	for _, unit := range liveUnits(t) {
		if slices.Contains(declaredCapabilities(t, filepath.Join(tierDir, unit.name)), "Reach") {
			t.Fatalf("extension %s now declares reach: extend the History metadata reader to its entity and column declarations", unit.name)
		}
	}
	if len(entities) == 0 {
		t.Fatal("no core reach metadata read")
	}
	return entities
}

func (r historyImageReader) metadataStrings(in historyImageArg, expr ast.Expr) ([]string, bool) {
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		typ := in.pkg.info.TypeOf(selector.X)
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = pointer.Elem()
		}
		if named, ok := typ.(*types.Named); ok && named.Obj().Pkg() != nil {
			path := named.Obj().Pkg().Path()
			if strings.HasSuffix(path, "/platform/sweepreach") && named.Obj().Name() == "Relinked" && selector.Sel.Name == "Entity" {
				return r.reachEntities, len(r.reachEntities) > 0
			}
			if strings.HasSuffix(path, "/modules/contacts") {
				switch named.Obj().Name() + "." + selector.Sel.Name {
				case "SignatureField.Name":
					return r.evidenceFields["contact"], len(r.evidenceFields["contact"]) > 0
				case "MeetingProfileField.Field":
					return r.evidenceFields["company"], len(r.evidenceFields["company"]) > 0
				}
			}
		}
	}
	// These scans return the company field_name protected by the same CHECK.
	name, ok := expr.(*ast.Ident)
	if !ok || in.decl == nil || !strings.HasSuffix(in.pkg.path, "/modules/contacts") {
		return nil, false
	}
	object := in.pkg.info.ObjectOf(name)
	scanned, constrained := false, false
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if len(call.Args) > 1 {
			var pieces []string
			ast.Inspect(call.Args[1], func(node ast.Node) bool {
				if literal, ok := node.(*ast.BasicLit); ok {
					if text, err := strconv.Unquote(literal.Value); err == nil {
						pieces = append(pieces, text)
					}
				}
				return true
			})
			sql := strings.Join(pieces, "")
			if strings.Contains(sql, "field_provenance") && strings.Contains(sql, "RETURNING f.object_id, f.field_name") && strings.Contains(sql, "object_type = 'company'") {
				constrained = true
			}
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Scan" || len(call.Args) < 2 {
			return true
		}
		address, ok := call.Args[1].(*ast.UnaryExpr)
		if !ok {
			return true
		}
		target, ok := address.X.(*ast.Ident)
		scanned = scanned || (ok && in.pkg.info.ObjectOf(target) == object)
		return true
	})
	return r.evidenceFields["company"], scanned && constrained && len(r.evidenceFields["company"]) > 0
}
