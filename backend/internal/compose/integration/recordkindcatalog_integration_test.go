// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

// kindTable creates one ext table for the verifier to read and drops it after.
func kindTable(t *testing.T, name, ddl string) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + name,
		`CREATE TABLE ext.` + name + ` (` + ddl + `)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + name + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing %s (%s): %v", name, stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+name); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
}

func declaredKind(table string, key ...string) recordkinds.Declared {
	return recordkinds.Declared{
		Name: "ext:kindcat:thing", Unit: "kindcat", Kind: "thing", Table: table,
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site"}},
		NaturalKey: key,
	}
}

func TestADeclaredKindMustHaveItsTableColumnsAndAUniqueNaturalKey(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	const good = `id uuid PRIMARY KEY, archived_at timestamptz, code text NOT NULL, site text, UNIQUE (code)`
	cases := []struct {
		name  string
		ddl   string
		table string
		key   []string
		want  string
	}{
		{"complete", good, "ext_kindcat_ok", []string{"code"}, ""},
		{"no table", "", "ext_kindcat_absent", []string{"code"}, "does not exist"},
		{"no archived_at", `id uuid PRIMARY KEY, code text UNIQUE, site text`, "ext_kindcat_noarchive", []string{"code"}, "archived_at"},
		{"no declared field", `id uuid PRIMARY KEY, archived_at timestamptz, code text UNIQUE`, "ext_kindcat_nofield", []string{"code"}, `"site"`},
		{"natural key not unique", `id uuid PRIMARY KEY, archived_at timestamptz, code text, site text`, "ext_kindcat_loose", []string{"code"}, "unique"},
		{"unique over the wrong columns", `id uuid PRIMARY KEY, archived_at timestamptz, code text, site text, UNIQUE (code, site)`, "ext_kindcat_wide", []string{"code"}, "unique"},
		{"unique only for some rows", `id uuid PRIMARY KEY, archived_at timestamptz, code text, site text`, "ext_kindcat_partial", []string{"code"}, "unique"},
		{"no rights for the application role", good, "ext_kindcat_nogrant", []string{"code"}, "lacks"},
		{"only one of the four rights", good, "ext_kindcat_selectonly", []string{"code"}, "lacks"},
	}
	for _, c := range cases {
		if c.ddl != "" {
			kindTable(t, c.table, c.ddl)
		}
		if c.name == "unique only for some rows" {
			if _, err := OwnerConn(t).Exec(ctx, `CREATE UNIQUE INDEX ext_kindcat_partial_code ON ext.ext_kindcat_partial (code) WHERE archived_at IS NULL`); err != nil {
				t.Fatal(err)
			}
		}
		if c.name == "no rights for the application role" {
			if _, err := OwnerConn(t).Exec(ctx, `REVOKE ALL ON ext.ext_kindcat_nogrant FROM margince_app`); err != nil {
				t.Fatal(err)
			}
		}
		if c.name == "only one of the four rights" {
			if _, err := OwnerConn(t).Exec(ctx, `REVOKE INSERT, UPDATE, DELETE ON ext.ext_kindcat_selectonly FROM margince_app`); err != nil {
				t.Fatal(err)
			}
		}
		err := compose.VerifyRecordKinds(ctx, env.Pool, []recordkinds.Declared{declaredKind(c.table, c.key...)})
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want one naming %q", c.name, err, c.want)
		}
	}
}
