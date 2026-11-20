// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "testing"

func TestOnlyAChangeThatCanMoveAGeneratorsOutputAsksForBothPasses(t *testing.T) {
	t.Parallel()
	p := plan{
		outputs:    []string{"backend/internal/contracts/types_gen.go"},
		goldenDirs: []string{"backend/gates"},
	}
	for _, c := range []struct {
		name    string
		changed []string
		want    string
	}{
		{"a module's own code", []string{"backend/internal/modules/deals/store.go", "frontend/src/app/home.tsx"}, ""},
		{"nothing at all", nil, ""},
		{"a generator's source", []string{"docs/a.md", "backend/tools/gen-config/main.go"}, "backend/tools/gen-config/main.go"},
		{"the contract the generators read", []string{"backend/api/crm.yaml"}, "backend/api/crm.yaml"},
		{"a generated file", []string{"backend/internal/contracts/types_gen.go"}, "backend/internal/contracts/types_gen.go"},
		{"a file beside a golden test", []string{"backend/gates/census_test.go"}, "backend/gates/census_test.go"},
		{"a folder under a golden test's package is another package", []string{"backend/gates/testdata/list.txt"}, ""},
		{"the recipe that runs them", []string{"backend/Makefile"}, "backend/Makefile"},
		{"a frontend generator script", []string{"frontend/scripts/gen-api.mjs"}, "frontend/scripts/gen-api.mjs"},
		{"a pinned library", []string{"extensions/openchannel/go.sum"}, "extensions/openchannel/go.sum"},
		{"a name that only starts like an input", []string{"scripts-notes/a.md", "backend/toolsmith/x.go"}, ""},
	} {
		got, hit := p.touchesGenerator(c.changed)
		if got != c.want || hit != (c.want != "") {
			t.Errorf("%s: touchesGenerator(%v) = %q, %v; want %q", c.name, c.changed, got, hit, c.want)
		}
	}
}
