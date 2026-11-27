// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pagedAsk is one deal or project list read: its size, where it starts, its
// order and the one owner it narrows to.
type pagedAsk struct {
	limit  int
	offset *int
	cursor *string
	sort   *string
	owner  *ids.UserID
}

// pagedList is a deal or project list, with a way to create one of its rows.
type pagedList struct {
	name   string
	sort   string
	create func(t *testing.T, owner ids.UUID, i int)
	read   func(ctx context.Context, ask pagedAsk) ([]ids.UUID, storekit.Page, error)
}

func pagedLists(t *testing.T, e *Env) []pagedList {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Paged customer", &e.Rep1)
	return []pagedList{{
		name: "deal", sort: "name",
		create: func(t *testing.T, owner ids.UUID, i int) {
			e.SeedDeal(t, fmt.Sprintf("Paged deal %d", i), pipeline, open, &owner)
		},
		read: func(ctx context.Context, ask pagedAsk) ([]ids.UUID, storekit.Page, error) {
			rows, page, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{
				Limit: &ask.limit, Offset: ask.offset, Cursor: ask.cursor, Sort: ask.sort, OwnerID: ask.owner,
			})
			out := make([]ids.UUID, 0, len(rows))
			for _, row := range rows {
				out = append(out, ids.UUID(row.Id))
			}
			return out, page, err
		},
	}, {
		name: "project", sort: "-name",
		create: func(t *testing.T, owner ids.UUID, i int) {
			seedProject(e.Admin(), t, e, fmt.Sprintf("Paged project %d", i), company, &owner)
		},
		read: func(ctx context.Context, ask pagedAsk) ([]ids.UUID, storekit.Page, error) {
			rows, page, err := e.Projects.ListProjects(ctx, projects.ListProjectsInput{
				Limit: &ask.limit, Offset: ask.offset, Cursor: ask.cursor, Sort: ask.sort, OwnerID: ask.owner,
			})
			return projectIDs(rows), page, err
		},
	}}
}

func projectIDs(rows []crmcontracts.Project) []ids.UUID {
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, ids.UUID(row.Id))
	}
	return out
}

func readPaged(ctx context.Context, t *testing.T, list pagedList, ask pagedAsk) ([]ids.UUID, storekit.Page) {
	t.Helper()
	rows, page, err := list.read(ctx, ask)
	if err != nil {
		t.Fatalf("listing %ss: %v", list.name, err)
	}
	return rows, page
}

func totalOfPage(t *testing.T, list pagedList, page storekit.Page) int {
	t.Helper()
	if page.Total == nil {
		t.Fatalf("the %s list answered no total on a page read without a cursor", list.name)
	}
	return *page.Total
}

func TestTheDealAndProjectListTotalsCountTheFilteredSet(t *testing.T) {
	e := Setup(t)
	for _, list := range pagedLists(t, e) {
		for i := range 5 {
			list.create(t, e.Rep1, i)
		}
		for i := range 3 {
			list.create(t, e.Rep2, 5+i)
		}
		_, whole := readPaged(e.Admin(), t, list, pagedAsk{limit: 2})
		if got := totalOfPage(t, list, whole); got != 8 {
			t.Errorf("%s total = %d, want all 8", list.name, got)
		}
		owner := ids.From[ids.UserKind](e.Rep2)
		_, narrowed := readPaged(e.Admin(), t, list, pagedAsk{limit: 2, owner: &owner})
		if got := totalOfPage(t, list, narrowed); got != 3 {
			t.Errorf("%s total for one owner = %d, want 3: the count dropped the filter", list.name, got)
		}
	}
}

// A seat reads every deal and every project, whoever owns it. A total built on
// the owner predicate would count only the reader's own rows.
func TestTheDealAndProjectListTotalsCountWhatTheReaderReads(t *testing.T) {
	e := Setup(t)
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"custom"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "project": {Read: true}, "company": {Read: true}, "custom_field": {Read: true},
		},
	})
	for _, list := range pagedLists(t, e) {
		for i := range 3 {
			list.create(t, e.Rep1, i)
		}
		list.create(t, e.Rep2, 3)
		list.create(t, e.Rep3, 4)

		_, first := readPaged(reader, t, list, pagedAsk{limit: 2})
		walked := 0
		for ask := (pagedAsk{limit: 2}); ; {
			rows, page := readPaged(reader, t, list, ask)
			walked += len(rows)
			if !page.HasMore {
				break
			}
			ask.cursor = &page.NextCursor
		}
		if got := totalOfPage(t, list, first); got != walked || walked != 5 {
			t.Errorf("%s total = %d over %d rows the reader pages through, want both 5", list.name, got, walked)
		}
	}
}

func TestTheDealAndProjectListsOpenAPageByOffset(t *testing.T) {
	e := Setup(t)
	for _, list := range pagedLists(t, e) {
		for i := range 5 {
			list.create(t, e.Rep1, i)
		}
		for _, sort := range []*string{nil, &list.sort} {
			whole, _ := readPaged(e.Admin(), t, list, pagedAsk{limit: 5, sort: sort})
			skipped, page := readPaged(e.Admin(), t, list, pagedAsk{limit: 2, offset: new(2), sort: sort})
			if !slices.Equal(skipped, whole[2:4]) || totalOfPage(t, list, page) != 5 || page.NextCursor == "" {
				t.Fatalf("%s offset 2 = %v %+v, want %v, total 5 and a cursor", list.name, skipped, page, whole[2:4])
			}
			rest, after := readPaged(e.Admin(), t, list, pagedAsk{limit: 2, cursor: &page.NextCursor, sort: sort})
			if !slices.Equal(rest, whole[4:]) || after.Total != nil {
				t.Errorf("%s after the offset page = %v total %v, want %v and no total", list.name, rest, after.Total, whole[4:])
			}
			past, beyond := readPaged(e.Admin(), t, list, pagedAsk{limit: 2, offset: new(7), sort: sort})
			if len(past) != 0 || totalOfPage(t, list, beyond) != 5 {
				t.Errorf("%s offset past the end = %d rows total %v, want none and total 5", list.name, len(past), beyond.Total)
			}

			for code, ask := range map[string]pagedAsk{
				"offset_with_cursor": {limit: 2, offset: new(1), cursor: &page.NextCursor, sort: sort},
				"out_of_range":       {limit: 2, offset: new(storekit.MaxListOffset + 1), sort: sort},
			} {
				_, _, err := list.read(e.Admin(), ask)
				var refused *storekit.OffsetError
				if !errors.As(err, &refused) || refused.Code != code {
					t.Errorf("%s %s: err = %v, want an OffsetError coded %s", list.name, code, err, code)
				}
			}
		}
	}
}
