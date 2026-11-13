// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textbounds

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

func TestTextPoliciesRequireAKnownLimitOrAReasonedException(t *testing.T) {
	for _, source := range []string{
		"public.company.description document\npublic.company.description note\n",
		"public.company.description arbitrary\n",
		"public.company.description exempt\n",
		"company.description note\n",
		"public.company.description label ignored\n",
		"public.company.description document checked:\n",
	} {
		if _, err := ParsePolicies(strings.NewReader(source)); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
	checked, err := ParsePolicies(strings.NewReader("public.company.description document checked: every writer refuses past the limit\n"))
	if err != nil || checked["public.company.description"] != (Policy{Limit: 20000, Checked: "every writer refuses past the limit"}) {
		t.Fatalf("a checked limit reads as %+v, %v", checked, err)
	}
	policies, err := Policies()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"public.company.description", "public.deal.description"} {
		if policies[key].Limit != 20000 {
			t.Fatalf("%s must admit the same document length", key)
		}
	}
	for _, key := range []string{"public.activity.body", "public.mail_draft.body", "public.mail_draft.html_body", "public.comms_outbound.body", "public.comms_outbound.html_body", "public.consent_text_version.body", "public.knowledge_chunk.text", "public.email_signature.body", "public.email_signature.html"} {
		if policies[key].Exception == "" {
			t.Errorf("%s must retain its complete source text", key)
		}
	}
}

func TestTheTextCensusIncludesRequiredAndGeneratedColumns(t *testing.T) {
	source := "public.activity.subject text NOT NULL gen=- def=''::text\npublic.company.description text gen=- def=-\npublic.company.search_title text gen=s def=expression\npublic.company.tags text[] NOT NULL gen=- def=-\n"
	columns, err := Columns(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 4 || columns[0].Generated || !columns[2].Generated || !columns[3].Array {
		t.Fatalf("incomplete text census: %+v", columns)
	}
	if _, err := Columns(strings.NewReader("public.company.id uuid NOT NULL gen=- def=-")); err == nil {
		t.Fatal("an empty census passed")
	}
}

func TestChangedTextGuardsPreserveExistingValues(t *testing.T) {
	guards, err := SQL(strings.NewReader("public.company.description text gen=- def=-\npublic.import_run.error text gen=- def=-\n"), map[string]bool{"import_run": true})
	if err != nil {
		t.Fatal(err)
	}
	up, down := guards.Functions.Up+guards.Tables.Up, guards.Tables.Down
	for _, fragment := range []string{"AFTER INSERT", "AFTER UPDATE", `text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"description":20000}')`, `text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"description":20000}')`, `IS DISTINCT FROM old_value COLLATE "C"`, `refuse_oversized_text('{"description":20000}')`, "CONSTRAINT = '" + storekit.TextBoundConstraint + "', COLUMN = bound.key, DETAIL = bound.value::text"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing changed-value guard %q", fragment)
		}
	}
	if !strings.Contains(down, `DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."company"`) {
		t.Fatal("rollback leaves a trigger behind")
	}
	if strings.Contains(up, `NEW."description"`) {
		t.Fatal("a trigger condition names a column, which blocks a later migration from dropping it")
	}
	if strings.Contains(up, `"import_run"`) || !strings.Contains(guards.Custom.Up, `ON "public"."import_run"`) || strings.Contains(guards.Custom.Up, "FUNCTION public.text_") {
		t.Fatal("a custom-track table must get its guards from the custom pair, which reuses the core functions")
	}
	if !strings.Contains(guards.Custom.Down, `DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."import_run"`) {
		t.Fatal("the custom rollback leaves a trigger behind")
	}
}

// One transaction would close every guarded table to writers until the last guard is
// made. A database that already has the guards must also take them again.
func TestEachTableGuardCommitsOnItsOwnAndRunsTwice(t *testing.T) {
	guards, err := SQL(strings.NewReader("public.company.description text gen=- def=-\npublic.deal.description text gen=- def=-\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for half, sql := range map[string]string{"up": guards.Tables.Up, "down": guards.Tables.Down} {
		if !(dbmigrate.Migration{UpSQL: sql}).Unwrapped() {
			t.Errorf("the %s table guards run in one transaction", half)
		}
		if !strings.HasSuffix(sql, "RESET lock_timeout;\n") {
			t.Errorf("the %s table guards leave the session lock wait set for later migrations", half)
		}
	}
	if strings.Count(guards.Tables.Up, "CREATE OR REPLACE TRIGGER") != 4 || strings.Contains(guards.Tables.Up, "CREATE TRIGGER") {
		t.Fatal("a table guard cannot run on a database that already has it")
	}
	if strings.Count(guards.Functions.Up, "CREATE OR REPLACE FUNCTION") != 4 || strings.Contains(guards.Functions.Up, "TRIGGER") {
		t.Fatal("the shared functions cannot run twice, or their pair locks a table")
	}
	if !strings.Contains(guards.CustomFields.Up, "CREATE OR REPLACE TRIGGER custom_text_bound_insert") || guards.Custom.Up != "" {
		t.Fatal("the custom-field guards cannot run on a database that already has them")
	}
}

func TestARetargetMovesTheGuardsToTheRegistryAndItsRollbackMovesThemBack(t *testing.T) {
	catalog := "public.activity.body text gen=- def=-\n" +
		"public.company.description text gen=- def=-\n" +
		`public.activity.text_bound_insert CREATE TRIGGER text_bound_insert AFTER INSERT ON public.activity FOR EACH ROW WHEN (text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"body": 20000}'::jsonb)) EXECUTE FUNCTION refuse_oversized_text('{"body": 20000}')` + "\n" +
		`public.company.text_bound_insert CREATE TRIGGER text_bound_insert AFTER INSERT ON public.company FOR EACH ROW WHEN (text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"description": 120}'::jsonb)) EXECUTE FUNCTION refuse_oversized_text('{"description": 120}')` + "\n"
	migration, err := Retarget(strings.NewReader(catalog), []string{"activity", "company"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(migration.Up, `TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity"`) ||
		!strings.Contains(migration.Up, `DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."activity"`) {
		t.Errorf("an exempt column's table keeps a guard:\n%s", migration.Up)
	}
	if !strings.Contains(migration.Up, `CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company"`) ||
		!strings.Contains(migration.Up, `'{"description":20000}'`) || !strings.Contains(migration.Down, `'{"description":120}'`) ||
		!strings.Contains(migration.Down, `'{"body":20000}'`) {
		t.Errorf("the pair does not move company to the registry and back:\nup:\n%s\ndown:\n%s", migration.Up, migration.Down)
	}
	if !(dbmigrate.Migration{UpSQL: migration.Up}).Unwrapped() || !strings.HasSuffix(migration.Down, "RESET lock_timeout;\n") {
		t.Error("a retarget locks every table it touches in one transaction")
	}
	if _, err := Retarget(strings.NewReader(catalog), []string{"deal"}); err == nil {
		t.Fatal("a table whose guards already match produced a migration")
	}
}

func TestFitCutsToTheColumnLimitAndKeepsExemptTextWhole(t *testing.T) {
	long := strings.Repeat("ế", 20001)
	if got := Fit("public.company.description", long); len([]rune(got)) != 20000 {
		t.Errorf("a document column kept %d characters, want 20000", len([]rune(got)))
	}
	if got := Fit("public.activity.body", long); got != long {
		t.Error("an exempt column's text was cut")
	}
}
