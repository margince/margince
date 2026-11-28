// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/compose/identifierspellings"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestEveryCheckedIdentifierColumnHasALegacyRepair(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`ALTER TABLE ([a-z_]+) ADD CONSTRAINT [a-z_]+ CHECK \(is_identifier_text\(([a-z_]+)\)\)`)
	checked := map[string]bool{}
	paths, err := gatekit.Glob("migrations/core/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(raw), -1) {
			checked[m[1]+"."+m[2]] = true
		}
	}
	if len(checked) < 12 {
		t.Fatalf("identifier constraint census is short: %v", checked)
	}
	for _, column := range identifierspellings.Columns() {
		key := column.Table + "." + column.Column
		if !checked[key] {
			t.Errorf("repair %s has no identifier constraint", key)
		}
		delete(checked, key)
		if tableOwners[column.Table] == "" {
			t.Errorf("repair %s has no table owner", key)
		}
	}
	for key := range checked {
		t.Errorf("checked identifier %s has no legacy repair", key)
	}
}
