// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textbounds

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestTextPoliciesRequireAKnownLimitOrAReasonedException(t *testing.T) {
	for _, source := range []string{
		"public.company.description document\npublic.company.description note\n",
		"public.company.description arbitrary\n",
		"public.company.description exempt\n",
		"company.description note\n",
		"public.company.description label ignored\n",
	} {
		if _, err := ParsePolicies(strings.NewReader(source)); err == nil {
			t.Errorf("accepted %q", source)
		}
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
	core, custom, err := SQL(strings.NewReader("public.company.description text gen=- def=-\npublic.import_run.error text gen=- def=-\n"), map[string]bool{"import_run": true})
	if err != nil {
		t.Fatal(err)
	}
	up, down := core.Up, core.Down
	for _, fragment := range []string{"AFTER INSERT", "AFTER UPDATE", `text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"description":20000}')`, `text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"description":20000}')`, `IS DISTINCT FROM old_value COLLATE "C"`, `refuse_oversized_text('{"description":20000}')`, "CONSTRAINT = '" + storekit.TextBoundConstraint + "', COLUMN = bound.key, DETAIL = bound.value::text"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing changed-value guard %q", fragment)
		}
	}
	if !strings.Contains(down, `DROP TRIGGER "text_bound_update" ON "public"."company"`) {
		t.Fatal("rollback leaves a trigger behind")
	}
	if strings.Contains(up, `NEW."description"`) {
		t.Fatal("a trigger condition names a column, which blocks a later migration from dropping it")
	}
	if strings.Contains(up, `"import_run"`) || !strings.Contains(custom.Up, `ON "public"."import_run"`) || strings.Contains(custom.Up, "CREATE FUNCTION") {
		t.Fatal("a custom-track table must get its guards from the custom pair, which reuses the core functions")
	}
	if !strings.Contains(custom.Down, `DROP TRIGGER "text_bound_insert" ON "public"."import_run"`) {
		t.Fatal("the custom rollback leaves a trigger behind")
	}
}
