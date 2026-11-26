// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every site a model request names is one api/ai-tasks.yaml declares.
//
// The router refuses an undeclared site at call time. Without this gate, a typo in
// Request.Site is a feature that fails on its first call.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// declaredSites maps the site names the task contract declares, read from the
// table generated from it.
func declaredSites() map[string]bool {
	sites := map[string]bool{}
	for _, task := range ai.AllTasks() {
		for _, site := range ai.SitesFor(task) {
			sites[site.Name] = true
		}
	}
	return sites
}

// requestSites is the Site of every model.Request composite literal in src,
// "" for one whose value is not a string this reader can resolve. It matches
// the literal, not an assignment to a field of one.
func requestSites(t *testing.T, path string, src []byte, constsOf func() map[string]string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	local, ok := localNameFor(file, "shared/ports/model")
	if !ok {
		return nil
	}
	consts := constsOf()
	var sites []string
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isRequestLiteral(literal, local) {
			return true
		}
		for _, element := range literal.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			if key, isKey := field.Key.(*ast.Ident); isField && isKey && key.Name == "Site" {
				site, _ := gatekit.StringExpr(field.Value, consts, gatekit.FoldStrict)
				sites = append(sites, site)
			}
		}
		return true
	})
	return sites
}

func TestEveryRequestSiteIsDeclaredInTheTaskContract(t *testing.T) {
	t.Parallel()
	declared := declaredSites()
	constsByDir := map[string]map[string]string{}
	constsOf := func(dir string) map[string]string {
		if _, read := constsByDir[dir]; !read {
			constsByDir[dir] = gatekit.PackageStringConstants(t, dir)
		}
		return constsByDir[dir]
	}
	named := 0
	err := gatekit.WalkDir("internal", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		for _, site := range requestSites(t, path, src, func() map[string]string { return constsOf(filepath.Dir(path)) }) {
			named++
			switch {
			case site == "":
				t.Errorf("%s: a model.Request names a Site this test cannot read; spell it as a string literal or a package constant", path)
			case !declared[site]:
				t.Errorf("%s: a model.Request names site %q, which no task in backend/api/ai-tasks.yaml declares", path, site)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if named == 0 {
		t.Fatal("no model.Request in the tree names a Site, so this census read nothing")
	}
}

// The census recognises each shape a site may be spelled in, and reports the
// one it cannot read rather than passing over it.
func TestTheRequestSiteCensusSeesEveryShape(t *testing.T) {
	t.Parallel()
	src := []byte(`package p
import port "github.com/margince/margince/backend/internal/shared/ports/model"
const named = "from_a_constant"
var _ = port.Request{Site: "literal"}
var _ = port.Request{Site: named}
var _ = port.Request{Site: computed()}
var _ = other.Request{Site: "not_a_model_request"}
`)
	got := requestSites(t, "planted.go", src, func() map[string]string { return map[string]string{"named": "from_a_constant"} })
	want := []string{"literal", "from_a_constant", ""}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("census read %q, want %q", got, want)
	}
}
