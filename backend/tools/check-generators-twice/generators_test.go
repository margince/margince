// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"reflect"
	"testing"
)

const driftRecipe = `gen: vocab ## Regenerate
	$(GOWORK_ROOT) $(GO) run ./tools/gen-composition $(COMPOSITION_FLAGS)
	$(GOWORK_ROOT) $(GO) generate ./internal/contracts

perfdoc: ## Pages
	@$(GOWORK_ROOT) $(GO) run ./tools/gen-perfdoc

extdoc-gen: ## Not in drift
	$(GOWORK_ROOT) $(GO) run ./tools/gen-extdoc

drift: ## Fail on drift
	@../scripts/keep-unchanged-mtimes.sh save ../.tmp/drift-mtimes
	$(MAKE) gen perfdoc
	git diff --exit-code
`

func TestTheMakeGeneratorIsWhatDriftRegenerates(t *testing.T) {
	g, targets, err := makeGenerator(driftRecipe)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.Argv, []string{"make", "gen", "perfdoc"}) || g.Dir != "backend" {
		t.Errorf("generator = %s %v, want make gen perfdoc in backend", g.Dir, g.Argv)
	}
	if tools := recipeTools(driftRecipe, targets); !reflect.DeepEqual(tools, []string{"gen-composition", "gen-perfdoc"}) {
		t.Errorf("tools = %v, want the two the drift targets run", tools)
	}
}

func TestADriftRecipeWithoutAMakeLineFailsTheCensus(t *testing.T) {
	if _, _, err := makeGenerator("drift:\n\tgit diff --exit-code\n"); err == nil {
		t.Error("a drift recipe that regenerates nothing passed the census")
	}
}

func TestEachFrontendScriptRunsOnceEvenWhenAnotherCallsIt(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{frontendPackage: `{"scripts": {
		"gen:api": "openapi-typescript a.yaml -o a.ts && pnpm gen:events",
		"gen:events": "openapi-typescript b.yaml -o b.ts",
		"gen:copy": "node scripts/copy.mjs",
		"build": "vite build"
	}}`, "frontend/scripts/copy.mjs": "writeFileSync(\"src/copy_gen.json\", copy);\n"})
	got, err := frontendGenerators(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range got {
		names = append(names, g.Name)
	}
	if want := []string{"pnpm run gen:api", "pnpm run gen:copy"}; !reflect.DeepEqual(names, want) {
		t.Errorf("frontend generators = %v, want %v", names, want)
	}
}

func TestAToolNamesAnOutputByAnyTrailingPartOfItsPath(t *testing.T) {
	named := map[string]string{"manifest.generated.json": "gen-composition", "frontend/src/stub.gen.ts": "gen-composition"}
	for output, want := range map[string]bool{
		"extensions/de/manifest.generated.json": true,
		"frontend/src/stub.gen.ts":              true,
		"other/src/stub.gen.ts":                 false,
	} {
		if _, got := namedBy(output, named); got != want {
			t.Errorf("namedBy(%s) = %v, want %v", output, got, want)
		}
	}
}

func TestOnlyTheOutputsASkippedScriptNamesGoUnchecked(t *testing.T) {
	skipped := []generator{{ScriptText: "openapi-typescript a.yaml -o src/api/schema.d.ts\n" +
		`join(root, "src/i18n/essential", locale + "_gen.json")` + "\n" +
		`join(root, "../backend/internal/modules/agents/previewcatalog_gen.json")`}}
	rest, owned := splitBySkipped([]string{
		"frontend/src/api/schema.d.ts", "frontend/src/i18n/essential/de_gen.json",
		"backend/internal/modules/agents/previewcatalog_gen.json", "backend/internal/compose/new_gen.go",
		"backend/internal/modules/agents/new_gen.go",
	}, skipped)
	want := []string{"backend/internal/compose/new_gen.go", "backend/internal/modules/agents/new_gen.go"}
	if !reflect.DeepEqual(rest, want) {
		t.Errorf("still required = %v, want %v", rest, want)
	}
	if len(owned) != 3 {
		t.Errorf("not checked = %v, want the three the script names", owned)
	}
}
