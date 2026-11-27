// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pageAsk is one list read: its size, and where it starts.
type pageAsk struct {
	limit  int
	offset *int
	cursor *string
	sort   *string
}

func readPage(ctx context.Context, store *Store, kind string, ask pageAsk) ([]ids.UUID, storekit.Page, error) {
	switch kind {
	case "contact":
		rows, page, err := store.ListContacts(ctx, ListContactsInput{Limit: &ask.limit, Offset: ask.offset, Cursor: ask.cursor, Sort: ask.sort})
		return rowIDs(rows, func(c crmcontracts.Contact) ids.UUID { return ids.UUID(c.Id) }), page, err
	case "company":
		rows, page, err := store.ListCompanies(ctx, ListCompaniesInput{Limit: &ask.limit, Offset: ask.offset, Cursor: ask.cursor, Sort: ask.sort})
		return rowIDs(rows, func(c crmcontracts.Company) ids.UUID { return ids.UUID(c.Id) }), page, err
	default:
		rows, page, err := store.ListLeads(ctx, ListLeadsInput{Limit: &ask.limit, Offset: ask.offset, Cursor: ask.cursor, Sort: ask.sort})
		return rowIDs(rows, func(l crmcontracts.Lead) ids.UUID { return ids.UUID(l.Id) }), page, err
	}
}

func rowIDs[T any](rows []T, id func(T) ids.UUID) []ids.UUID {
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, id(row))
	}
	return out
}

// offsetLists are the record lists that take an offset, each under its default
// order and under a sort the client chose. The lead list with no sort is the
// work queue, which builds its own page.
var offsetLists = []struct {
	kind string
	sort *string
}{
	{"contact", nil},
	{"contact", new("-full_name")},
	{"company", nil},
	{"company", new("display_name")},
	{"lead", nil},
	{"lead", new("-full_name")},
}

func TestAnOffsetPageStartsThatManyRowsIntoTheList(t *testing.T) {
	for _, list := range offsetLists {
		t.Run(fmt.Sprintf("%s/sort=%v", list.kind, list.sort != nil), func(t *testing.T) {
			e := setupCapturePrivacy(t)
			ctx := e.as(e.owner, principal.RowScopeAll)
			const seeded = 7
			for i := range seeded {
				createTotalRecord(ctx, t, e.store, list.kind, i)
			}
			read := func(ask pageAsk) ([]ids.UUID, storekit.Page) {
				t.Helper()
				ask.sort = list.sort
				rows, page, err := readPage(ctx, e.store, list.kind, ask)
				if err != nil {
					t.Fatal(err)
				}
				return rows, page
			}
			whole, _ := read(pageAsk{limit: seeded})
			if len(whole) != seeded {
				t.Fatalf("the whole list holds %d rows, want %d", len(whole), seeded)
			}

			skipped, page := read(pageAsk{limit: 2, offset: new(3)})
			if !slices.Equal(skipped, whole[3:5]) {
				t.Fatalf("offset 3 read %v, want rows 4 and 5 of %v", skipped, whole)
			}
			if page.Total == nil || *page.Total != seeded || !page.HasMore || page.NextCursor == "" {
				t.Fatalf("offset page = %+v, want total %d and a cursor to the rest", page, seeded)
			}
			rest, after := read(pageAsk{limit: 2, cursor: &page.NextCursor})
			if !slices.Equal(rest, whole[5:7]) {
				t.Fatalf("the cursor from an offset page read %v, want %v: a gap or a repeat", rest, whole[5:7])
			}
			if after.Total != nil {
				t.Fatalf("a page read through a cursor sent total %d", *after.Total)
			}
		})
	}
}

func TestAnOffsetPageCountsTheWholeListEvenPastItsEnd(t *testing.T) {
	for _, list := range offsetLists {
		t.Run(fmt.Sprintf("%s/sort=%v", list.kind, list.sort != nil), func(t *testing.T) {
			e := setupCapturePrivacy(t)
			ctx := e.as(e.owner, principal.RowScopeAll)
			const seeded = 5
			for i := range seeded {
				createTotalRecord(ctx, t, e.store, list.kind, i)
			}
			for _, c := range []struct {
				offset, rows int
			}{{1, 2}, {3, 2}, {4, 1}, {5, 0}, {9, 0}} {
				got, page, err := readPage(ctx, e.store, list.kind, pageAsk{limit: 2, offset: new(c.offset), sort: list.sort})
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != c.rows {
					t.Errorf("offset %d read %d rows, want %d", c.offset, len(got), c.rows)
				}
				if page.Total == nil || *page.Total != seeded {
					t.Errorf("offset %d total = %v, want %d: an offset page counts like a first page", c.offset, page.Total, seeded)
				}
			}
		})
	}
}

func TestAnOffsetIsRefusedBesideACursorAndPastTheCap(t *testing.T) {
	for _, list := range offsetLists {
		t.Run(fmt.Sprintf("%s/sort=%v", list.kind, list.sort != nil), func(t *testing.T) {
			e := setupCapturePrivacy(t)
			ctx := e.as(e.owner, principal.RowScopeAll)
			createTotalRecord(ctx, t, e.store, list.kind, 0)
			createTotalRecord(ctx, t, e.store, list.kind, 1)
			_, first, err := readPage(ctx, e.store, list.kind, pageAsk{limit: 1, sort: list.sort})
			if err != nil || first.NextCursor == "" {
				t.Fatalf("first page = %+v, %v; want a cursor", first, err)
			}
			for name, ask := range map[string]pageAsk{
				"offset_with_cursor": {limit: 1, offset: new(1), cursor: &first.NextCursor, sort: list.sort},
				"out_of_range":       {limit: 1, offset: new(storekit.MaxListOffset + 1), sort: list.sort},
			} {
				_, _, err := readPage(ctx, e.store, list.kind, ask)
				var refused *storekit.OffsetError
				if !errors.As(err, &refused) || refused.Code != name {
					t.Errorf("%s: err = %v, want an OffsetError coded %s", name, err, name)
				}
			}
		})
	}
}
