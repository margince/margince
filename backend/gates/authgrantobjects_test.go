// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The objects platform/auth asks for by name are objects a role can hold.
//
// platform sits below modules in the DAG, so platform/auth cannot import the
// policy vocabulary it checks grants against, and names a few objects itself
// (team_lead, team_oversight). Rename or drop one in policy and the gate in
// platform keeps asking for the old name, which no role holds: every seat is
// refused, and the refusal reads as a correctly denied request.
//
// Derived from both sides: every `obj… = "…"` constant declared in
// platform/auth, against coreObjects parsed out of policy.go.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheObjectsPlatformAuthNamesAreCoreObjects(t *testing.T) {
	t.Parallel()

	named := authObjectConstants(t)
	if len(named) == 0 {
		t.Fatal("read no obj… constants out of platform/auth — the declarations this gate reads have moved")
	}
	core := coreObjectsFromSource(t)
	for name, object := range named {
		if !slices.Contains(core, object) {
			t.Errorf("platform/auth's %s asks for %q, which policy.coreObjects does not declare — "+
				"no role can hold it, so the gate it guards refuses everybody", name, object)
		}
	}
}

// authObjectConstants reads every string constant named obj… in platform/auth.
func authObjectConstants(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join("internal", "platform", "auth")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		for _, decl := range file.Decls {
			collectObjectConstants(t, decl, out)
		}
	}
	return out
}

func collectObjectConstants(t *testing.T, decl ast.Decl, out map[string]string) {
	t.Helper()
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.CONST {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range value.Names {
			if !strings.HasPrefix(name.Name, "obj") || i >= len(value.Values) {
				continue
			}
			lit, ok := value.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			object, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("reading %s's value: %v", name.Name, err)
			}
			out[name.Name] = object
		}
	}
}
