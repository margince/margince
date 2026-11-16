// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestSearchDataAffordanceMirrorsTheServerReadGate(t *testing.T) {
	t.Parallel()
	read := func(path string) string {
		body, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	server := regexp.MustCompile(`(?s)func requireSearchDataRead.*?\[\]string\{([^}]+)\}`).FindStringSubmatch(read("internal/compose/searchdata.go"))
	client := regexp.MustCompile(`(?s)const readData\s*=([^;]+);`).FindStringSubmatch(read("../frontend/src/app/paletteanswer.tsx"))
	if len(server) != 2 || len(client) != 2 {
		t.Fatal("the search read-gate census found no policy")
	}
	var backendObjects, frontendObjects []string
	mandatory := regexp.MustCompile(`auth.Require\(ctx,\s*"([a-z_]+)",\s*principal.ActionRead\)`).FindStringSubmatch(read("internal/compose/searchdata.go"))
	if len(mandatory) != 2 {
		t.Fatal("search has no declared installation read gate")
	}
	backendObjects = append(backendObjects, mandatory[1])
	objects, err := parser.ParseExpr("[]string{" + server[1] + "}")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := objects.(*ast.CompositeLit)
	if !ok {
		t.Fatal("the search object census found no list")
	}
	constants := gatekit.PackageStringConstants(t, "internal/compose")
	for _, element := range list.Elts {
		object, known := gatekit.StringExpr(element, constants, gatekit.FoldStrict)
		if !known {
			t.Fatal("the search object census cannot resolve an entry")
		}
		backendObjects = append(backendObjects, object)
	}
	calls := regexp.MustCompile(`grants\(me\.data,\s*"([a-z_]+)",\s*"read"\)`)
	for _, match := range calls.FindAllStringSubmatch(client[1], -1) {
		frontendObjects = append(frontendObjects, match[1])
	}
	slices.Sort(backendObjects)
	slices.Sort(frontendObjects)
	if len(backendObjects) == 0 || !slices.Equal(backendObjects, frontendObjects) {
		t.Fatalf("server grants %v differ from palette grants %v", backendObjects, frontendObjects)
	}
}
