// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sweepreach

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

func TestTheEraseStatementNumbersItsPlaceholdersFromItsArguments(t *testing.T) {
	t.Parallel()
	pair := []reach.Link{{Column: "record_id", KindColumn: "record_type"}}
	for _, c := range []struct {
		name     string
		owned    Owned
		want     string
		argCount int
	}{
		{
			"tombstone takes the second placeholder",
			Owned{Owner: "a", Table: reach.Table{Name: "note", Links: pair, Erase: []reach.Clear{
				{Column: "body", To: reach.Tombstone}, {Column: "hint", To: reach.Empty}, {Column: "record_id", To: reach.Null},
			}}},
			`UPDATE "note" SET "body" = $2, "hint" = '', "record_id" = NULL WHERE ("record_type" = 'contact' AND "record_id" = $1) RETURNING id`,
			2,
		},
		{
			"no tombstone, no second argument",
			Owned{Owner: "a", Schema: "ext", Table: reach.Table{Name: "ext_a_note", Links: []reach.Link{
				{Column: "contact_id", Record: reach.Contact}, {Column: "company_id", Record: reach.Company},
			}, Erase: []reach.Clear{{Column: "body", To: reach.Null}}}},
			`UPDATE "ext"."ext_a_note" SET "body" = NULL WHERE "contact_id" = $1 RETURNING id`,
			1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, args := eraseStatement(c.owned, ids.NewV7(), "erased")
			if got != c.want || len(args) != c.argCount {
				t.Fatalf("eraseStatement() = %q with %d args, want %q with %d", got, len(args), c.want, c.argCount)
			}
		})
	}
}

func TestASetIsRefusedWholeWhenOneEntryIsWrong(t *testing.T) {
	company := reach.Table{Name: "invoice", Links: []reach.Link{{Column: "company_id", Record: reach.Company}}}
	for _, c := range []struct {
		name string
		set  []Owned
		want string
	}{
		{"one table, two owners", []Owned{{Owner: "a", Table: company}, {Owner: "b", Table: company}}, "both a and b"},
		{"another schema", []Owned{{Owner: "a", Schema: "public", Table: company}}, "core or ext"},
		{"invalid table", []Owned{{Owner: "a", Table: reach.Table{Name: "invoice"}}}, "declares no link"},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := installed.tables
			err := Install(c.set)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Install() = %v, want an error containing %q", err, c.want)
			}
			if len(installed.tables) != len(before) {
				t.Fatal("a refused set changed the installed one")
			}
		})
	}
}

// A process that never installed the set refuses to merge or erase, rather
// than running over no tables and reporting success.
func TestAnUnsealedSetRefusesTheMergeAndTheErasure(t *testing.T) {
	installed.mu.Lock()
	sealed := installed.sealed
	installed.sealed = false
	installed.mu.Unlock()
	t.Cleanup(func() {
		installed.mu.Lock()
		installed.sealed = sealed
		installed.mu.Unlock()
	})

	if err := Relink(context.Background(), nil, reach.Company, ids.NewV7(), ids.NewV7()); !errors.Is(err, ErrNotSealed) {
		t.Errorf("Relink() = %v, want ErrNotSealed", err)
	}
	if _, err := Erase(context.Background(), nil, ids.NewV7(), "erased"); !errors.Is(err, ErrNotSealed) {
		t.Errorf("Erase() = %v, want ErrNotSealed", err)
	}
}

// The dependents check of a merge skips exactly the columns Relink moves, so
// the two must agree per record kind.
func TestTheMovedColumnsAreTheOnesRelinkMovesForThatKind(t *testing.T) {
	installed.mu.Lock()
	tables, sealed := installed.tables, installed.sealed
	installed.mu.Unlock()
	t.Cleanup(func() {
		installed.mu.Lock()
		installed.tables, installed.sealed = tables, sealed
		installed.mu.Unlock()
	})
	if err := Install([]Owned{{Owner: "a", Schema: "ext", Table: reach.Table{Name: "ext_a_note", Links: []reach.Link{
		{Column: "company_id", Record: reach.Company},
		{Column: "contact_id", Record: reach.Contact},
		{Column: "record_id", KindColumn: "record_type"},
	}}}}); err != nil {
		t.Fatal(err)
	}
	column := func(name string) Column { return Column{Schema: "ext", Table: "ext_a_note", Name: name} }

	for record, want := range map[reach.Record][]Column{
		reach.Company: {column("company_id"), column("record_id")},
		reach.Contact: {column("contact_id"), column("record_id")},
		reach.Lead:    {column("record_id")},
	} {
		moved, err := MovedColumns(record)
		if err != nil {
			t.Fatal(err)
		}
		if len(moved) != len(want) {
			t.Errorf("a %s merge moves %v, want %v", record, moved, want)
		}
		for _, c := range want {
			if !moved[c] {
				t.Errorf("a %s merge does not move %s.%s", record, c.Table, c.Name)
			}
		}
	}
	if _, err := MovedColumns("activity"); err == nil {
		t.Error("MovedColumns accepted a record kind no merge moves")
	}
}
