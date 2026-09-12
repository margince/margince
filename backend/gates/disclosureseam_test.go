// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The two halves of the disclosure seam describe the same thing.
//
// consent owns the jurisdiction packs and renders each obligation into words;
// activities composes the body and appends them. They are siblings and neither
// may import the other, so each declares its own DisclosureLine and the
// composition root converts between them.
//
// A drift is quiet and expensive. Adding a field on the consent side without
// the activities side would drop it at the seam, and the adapter would keep
// compiling because it names the fields it copies — so the obligation would be
// rendered, carried across a boundary that silently forgot it, and left out of
// the message. The register in messagingruleapplied_test.go would still read as
// closed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestTheDisclosureLineAgreesAcrossTheSeam holds the claim in
// activities/disclosurefooter.go's DisclosureLine doc comment.
func TestTheDisclosureLineAgreesAcrossTheSeam(t *testing.T) {
	t.Parallel()
	owner := structFields(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "consent", "disclosurerender.go"),
		"DisclosureLine")
	mirror := structFields(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "activities", "disclosurefooter.go"),
		"DisclosureLine")

	if strings.Join(owner, ",") != strings.Join(mirror, ",") {
		t.Errorf("consent's DisclosureLine carries %v and activities' carries %v.\n\n"+
			"The compose adapter copies field by field and keeps compiling when one side "+
			"grows, so a field added on the consent side is rendered, dropped at the seam, "+
			"and left out of the message — while the unapplied-obligation register still "+
			"reads as closed.", owner, mirror)
	}
}

// structFields answers the named struct's exported field names, sorted.
func structFields(t *testing.T, path, name string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, isSpec := n.(*ast.TypeSpec)
		if !isSpec || spec.Name.Name != name {
			return true
		}
		st, isStruct := spec.Type.(*ast.StructType)
		if !isStruct {
			return true
		}
		for _, field := range st.Fields.List {
			// AN EMBEDDED FIELD CARRIES NO Names, so reading Names alone would
			// let one side gain an embedded struct — and every exported field
			// inside it — while this gate reported the two shapes identical.
			// It is named by its type instead, which is the name a caller
			// writes to reach it.
			if len(field.Names) == 0 {
				if name := embeddedName(field.Type); name != "" {
					out = append(out, name)
				}
				continue
			}
			for _, ident := range field.Names {
				if ident.IsExported() {
					out = append(out, ident.Name)
				}
			}
		}
		return false
	})
	if len(out) == 0 {
		t.Fatalf("%s declares no exported fields on %s — this gate is reading a shape that "+
			"is gone; point it at what replaced it", path, name)
	}
	sort.Strings(out)
	return out
}

// embeddedName answers the field name an embedded type contributes.
//
// A pointer embed reaches its fields the same way a value embed does, so both
// are named; an unexported embedded type contributes no exported surface and
// is skipped.
func embeddedName(expr ast.Expr) string {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	switch t := expr.(type) {
	case *ast.Ident:
		if t.IsExported() {
			return t.Name
		}
	case *ast.SelectorExpr:
		if t.Sel.IsExported() {
			return t.Sel.Name
		}
	}
	return ""
}
