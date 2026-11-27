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
	t.Parallel()
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
		{"no trigram index for search", good, "ext_kindcat_notrgm", []string{"code"}, "trigram"},
		{"a trigram index over some rows only", good, "ext_kindcat_parttrgm", []string{"code"}, "trigram"},
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
		if c.name == "complete" {
			titleIndex(t, c.table)
		}
		if c.name == "a trigram index over some rows only" {
			if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.ext_kindcat_parttrgm USING gin (lower(code) public.gin_trgm_ops) WHERE false`); err != nil {
				t.Fatal(err)
			}
		}
		if c.name == "unique only for some rows" {
			if _, err := OwnerConn(t).Exec(ctx, `CREATE UNIQUE INDEX ext_kindcat_partial_code ON ext.ext_kindcat_partial (code) WHERE archived_at IS NULL`); err != nil {
				t.Fatal(err)
			}
		}
		if c.name == "no rights for the application role" {
			if _, err := OwnerConn(t).Exec(ctx, `REVOKE ALL ON ext.ext_kindcat_nogrant FROM margince_app, ext_runtime`); err != nil {
				t.Fatal(err)
			}
		}
		if c.name == "only one of the four rights" {
			if _, err := OwnerConn(t).Exec(ctx, `REVOKE INSERT, UPDATE, DELETE ON ext.ext_kindcat_selectonly FROM margince_app, ext_runtime`); err != nil {
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

func TestADeclaredKindTimelineColumnsMustExistWithTheRightTypes(t *testing.T) {
	t.Parallel()
	env := Setup(t)
	ctx := context.Background()
	const base = `id uuid PRIMARY KEY, archived_at timestamptz, code text NOT NULL UNIQUE, site text, `
	cases := []struct {
		name, table, ddl, at, subject, want string
	}{
		{"complete", "ext_kindcat_tlok", base + `seen_at timestamptz, contact_id uuid`, "seen_at", "contact_id", ""},
		{"missing time column", "ext_kindcat_tlnotime", base + `contact_id uuid`, "seen_at", "contact_id", "seen_at"},
		{"missing subject column", "ext_kindcat_tlnosubj", base + `seen_at timestamptz`, "seen_at", "contact_id", "contact_id"},
		{"time column is text", "ext_kindcat_tltext", base + `seen_at text, contact_id uuid`, "seen_at", "contact_id", "not timestamp with time zone"},
		{"subject column is text", "ext_kindcat_tlsubj", base + `seen_at timestamptz, contact_id text`, "seen_at", "contact_id", "not uuid"},
	}
	for _, c := range cases {
		kindTable(t, c.table, c.ddl)
		if c.want == "" {
			titleIndex(t, c.table)
			if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.`+c.table+` (`+c.subject+`, `+c.at+`)`); err != nil {
				t.Fatal(err)
			}
		}
		k := declaredKind(c.table, "code")
		k.Timeline = &extension.RecordKindTimeline{
			LabelField: "code", AtColumn: c.at,
			Subjects: []extension.TimelineSubject{{Entity: "contact", Column: c.subject}},
		}
		err := compose.VerifyRecordKinds(ctx, env.Pool, []recordkinds.Declared{k})
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want one naming %q", c.name, err, c.want)
		}
	}
}

func TestADeclaredKindTitleColumnMustBeText(t *testing.T) {
	t.Parallel()
	env := Setup(t)
	kindTable(t, "ext_kindcat_titleint", `id uuid PRIMARY KEY, archived_at timestamptz, code integer NOT NULL UNIQUE, site text`)
	err := compose.VerifyRecordKinds(context.Background(), env.Pool, []recordkinds.Declared{declaredKind("ext_kindcat_titleint", "code")})
	if err == nil || !strings.Contains(err.Error(), "title column") {
		t.Errorf("err = %v, want one naming the title column", err)
	}
}

// titleIndex adds the trigram index search needs over the kind's title column,
// which for declaredKind is its natural key, code.
func titleIndex(t *testing.T, table string) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(),
		`CREATE INDEX ON ext.`+table+` USING gin (lower(code) public.gin_trgm_ops)`); err != nil {
		t.Fatal(err)
	}
}

func TestAKindWithAHistoryNeedsAnIndexPerSubject(t *testing.T) {
	t.Parallel()
	env := Setup(t)
	ctx := context.Background()
	const table = "ext_kindcat_timeline"
	kindTable(t, table, `id uuid PRIMARY KEY, archived_at timestamptz, code text NOT NULL UNIQUE, site text,
		visited_at timestamptz, contact_id uuid, deal_id uuid`)
	titleIndex(t, table)
	kind := declaredKind(table, "code")
	kind.Timeline = &extension.RecordKindTimeline{LabelField: "code", AtColumn: "visited_at", Subjects: []extension.TimelineSubject{
		{Entity: "contact", Column: "contact_id"}, {Entity: "deal", Column: "deal_id"},
	}}
	verify := func() error { return compose.VerifyRecordKinds(ctx, env.Pool, []recordkinds.Declared{kind}) }

	if err := verify(); err == nil || !strings.Contains(err.Error(), "contact History") {
		t.Fatalf("with no History index: err = %v, want one naming the contact History", err)
	}
	if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.`+table+` (visited_at, contact_id)`); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err == nil || !strings.Contains(err.Error(), "contact History") {
		t.Fatalf("with the columns the wrong way round: err = %v, want one naming the contact History", err)
	}
	if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.`+table+` (contact_id, visited_at) WHERE false`); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err == nil || !strings.Contains(err.Error(), "contact History") {
		t.Fatalf("with an index over some rows only: err = %v, want one naming the contact History", err)
	}
	if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.`+table+` (contact_id, visited_at)`); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err == nil || !strings.Contains(err.Error(), "deal History") {
		t.Fatalf("with only the contact index: err = %v, want one naming the deal History", err)
	}
	if _, err := OwnerConn(t).Exec(ctx, `CREATE INDEX ON ext.`+table+` (deal_id, visited_at DESC)`); err != nil {
		t.Fatal(err)
	}
	if err := verify(); err != nil {
		t.Errorf("with both History indexes: %v", err)
	}
}
