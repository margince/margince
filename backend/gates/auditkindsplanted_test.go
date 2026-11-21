// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The two readers behind the audit kinds gate, on planted migrations and
// planted TypeScript: each shape is read, or refused rather than guessed.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTheMigrationAuditReaderReadsOrRefusesEachShape(t *testing.T) {
	t.Parallel()
	const columns = "INSERT INTO audit_log (actor_type, action, entity_type, entity_id)"
	cases := []struct {
		name, sql string
		want      []string // nil means the statement is refused as opaque
	}{
		{"an INSERT with no column list is refused", "INSERT INTO audit_log VALUES ('system', 'update', 'deal', gen_random_uuid());", nil},
		{"a CTE arm is refused", columns + " SELECT 'system', 'update', 'deal', id FROM a UNION ALL (WITH s AS (SELECT 1) SELECT 'system', 'update', kind, id FROM s);", nil},
		{"a CASE with a column arm is refused", columns + " SELECT 'system', 'update', CASE WHEN x THEN 'deal' ELSE kind END, id FROM t;", nil},
		{"a CASE of literals is read", columns + " SELECT 'system', 'update', CASE WHEN x THEN 'deal' ELSE 'lead' END, id FROM t;", []string{"deal", "lead"}},
		{"ON CONFLICT names no row", "INSERT INTO audit_log (actor_type, action, entity_id, entity_type) VALUES ('system', 'update', gen_random_uuid(), 'deal') ON CONFLICT DO NOTHING;", []string{"deal"}},
		{"a semicolon inside a string does not end the statement", columns + " VALUES ('system', 'a;b', 'deal', gen_random_uuid());", []string{"deal"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "1_planted.up.sql"), []byte(tc.sql+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			types, opaque := auditMigrationTypes(t, root)
			got := sortedKeys(types)
			if tc.want == nil {
				if len(opaque) != 1 || len(got) != 0 {
					t.Errorf("read %v and refused %v, want the statement refused", got, opaque)
				}
				return
			}
			if len(opaque) != 0 || !slices.Equal(got, tc.want) {
				t.Errorf("read %v and refused %v, want %v", got, opaque, tc.want)
			}
		})
	}
}

func TestTheAuditKindLabelsReaderRefusesAnItemItCannotRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(name, source string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	kinds := write("audit.kinds.ts", `export const AUDIT_KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  deal_room: "audit.kind.deal_room",
};
`)
	search := write("searchkinds.ts", `export const SEARCH_HIT_KIND_KEY = {
  contact: "search.kind.contact",
};
`)
	page := write("audit.tsx", `const KIND_LABELS = new Map<string, MessageKey>([
  ...Object.entries(SEARCH_HIT_KIND_KEY),
  ...Object.entries(AUDIT_KIND_LABEL),
  ["custom_field", "audit.kind.customField"],
  ...extraLabels(),
]);
`)

	worded, owned, problems := auditKindLabels(t, page, kinds, search)

	if want := []string{"contact", "custom_field", "deal_room"}; !slices.Equal(sortedKeys(worded), want) {
		t.Errorf("worded %v, want %v", sortedKeys(worded), want)
	}
	if want := []string{"custom_field", "deal_room"}; !slices.Equal(sortedKeys(owned), want) {
		t.Errorf("owned %v, want %v: the search kinds are worded by search, not owned here", sortedKeys(owned), want)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "extraLabels") {
		t.Errorf("problems %q, want the unread spread refused", problems)
	}
}
