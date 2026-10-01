// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobject

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

func attachments() Reference {
	return Reference{
		Kind:    "attachment",
		Columns: []Column{{Table: "attachment", Name: "storage_key"}},
		Grace:   time.Hour,
	}
}

func TestADeclarationTheReapCouldNotReadSafelyIsRefused(t *testing.T) {
	t.Parallel()
	cases := map[string][]Reference{
		"nothing declared":        nil,
		"an empty kind":           {{Kind: "", Grace: time.Hour, Columns: attachments().Columns}},
		"a kind spanning '/'":     {{Kind: "a/b", Grace: time.Hour, Columns: attachments().Columns}},
		"no grace":                {{Kind: "attachment", Columns: attachments().Columns}},
		"no column":               {{Kind: "attachment", Grace: time.Hour}},
		"a column with no table":  {{Kind: "attachment", Grace: time.Hour, Columns: []Column{{Name: "storage_key"}}}},
		"one kind declared twice": {attachments(), attachments()},
	}
	for name, refs := range cases {
		if _, err := NewLedger(nil, refs...); !errors.Is(err, errInvalidReference) {
			t.Errorf("%s: NewLedger answered %v, want the declaration refused", name, err)
		}
	}
}

func TestTheOrphanReadBindsOneArgumentPerPlaceholder(t *testing.T) {
	t.Parallel()
	logos := Reference{Kind: "company_logo", Grace: 24 * time.Hour, Columns: []Column{
		{Table: "company", Name: "logo_object_key"}, {Table: "site_read", Name: "logo_object_key"},
	}}
	ledger, err := NewLedger(nil, attachments(), logos)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	statement, args := ledger.orphansQuery(now, 50)

	placeholders := map[string]bool{}
	for _, p := range regexp.MustCompile(`\$\d+`).FindAllString(statement, -1) {
		placeholders[p] = true
	}
	if len(placeholders) != len(args) {
		t.Fatalf("%d distinct placeholders against %d arguments:\n%s", len(placeholders), len(args), statement)
	}
	// Each kind's cutoff is its OWN grace: a shared one would either delete a
	// slow kind's uploads in flight or keep a fast kind's orphans a day.
	want := []any{"attachment", now.Add(-time.Hour), "company_logo", now.Add(-24 * time.Hour), 50}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("argument $%d is %v, want %v", i+1, args[i], want[i])
		}
	}
	consulted := []string{
		`"attachment" r WHERE r."storage_key"`,
		`"company" r WHERE r."logo_object_key"`,
		`"site_read" r WHERE r."logo_object_key"`,
	}
	for _, col := range consulted {
		if !strings.Contains(statement, col) {
			t.Errorf("the read never consults %s, so a live object there would be deleted:\n%s", col, statement)
		}
	}
}
