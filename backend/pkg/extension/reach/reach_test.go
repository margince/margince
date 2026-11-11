// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reach

import (
	"strings"
	"testing"
)

func TestADeclarationIsRefusedWhenTheCoreCannotActOnIt(t *testing.T) {
	t.Parallel()
	company := []Link{{Column: "company_id", Record: Company}}
	for _, c := range []struct {
		name  string
		table Table
		want  string
	}{
		{"bad table name", Table{Name: "Ext-Notes", Links: company}, "not an identifier"},
		{"no link", Table{Name: "ext_a_note"}, "declares no link"},
		{"no kind at all", Table{Name: "ext_a_note", Links: []Link{{Column: "record_id"}}}, "names no record kind"},
		{"both kinds", Table{Name: "ext_a_note", Links: []Link{{Column: "record_id", Record: Contact, KindColumn: "record_type"}}}, "both"},
		{"unknown kind", Table{Name: "ext_a_note", Links: []Link{{Column: "deal_id", Record: "deal"}}}, "not one the merge moves"},
		{"row id as link", Table{Name: "ext_a_note", Links: []Link{{Column: "id", Record: Company}}}, "not an identifier"},
		{"erase without a contact link", Table{Name: "ext_a_note", Links: company, Erase: []Clear{{Column: "body", To: Null}}}, "no link can hold a contact"},
		{"erase the row id", Table{Name: "ext_a_note", Links: []Link{{Column: "contact_id", Record: Contact}}, Erase: []Clear{{Column: "id", To: Null}}}, "may clear"},
		{"erase twice", Table{Name: "ext_a_note", Links: []Link{{Column: "contact_id", Record: Contact}}, Erase: []Clear{{Column: "body", To: Null}, {Column: "body", To: Empty}}}, "twice"},
		{"unknown value", Table{Name: "ext_a_note", Links: []Link{{Column: "contact_id", Record: Contact}}, Erase: []Clear{{Column: "body", To: "zero"}}}, "not null, empty or tombstone"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.table.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, c.want)
			}
		})
	}
}

func TestADeclarationTheCoreCanActOnIsAccepted(t *testing.T) {
	t.Parallel()
	for _, table := range []Table{
		{Name: "ext_a_invoice", Links: []Link{{Column: "company_id", Record: Company}}},
		{Name: "ext_a_note", Links: []Link{{Column: "record_id", KindColumn: "record_type"}}, Erase: []Clear{
			{Column: "body", To: Tombstone}, {Column: "author_note", To: Empty}, {Column: "record_id", To: Null},
		}},
	} {
		if err := table.Validate(); err != nil {
			t.Errorf("Validate(%s) = %v, want nil", table.Name, err)
		}
	}
}
