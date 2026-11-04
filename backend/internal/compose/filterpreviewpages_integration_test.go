// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Paging a filter preview. The cursor walks every match once in id order, and
// each page carries the full count. A reader meets no row they may not see.

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var previewPagedFilter = storekit.Predicate{Field: "name", Op: "contains", Value: "Paged Subject"}

// seedPagedContacts writes n contacts owned by owner through the real store.
func seedPagedContacts(t *testing.T, e *integration.Env, owner ids.UUID, n int) []ids.UUID {
	t.Helper()
	stamped := ids.From[ids.UserKind](owner)
	created := make([]ids.UUID, 0, n)
	for i := range n {
		contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: fmt.Sprintf("Paged Subject %s %d", owner, i), Source: "manual", OwnerID: &stamped,
		})
		if err != nil {
			t.Fatalf("seed contact %d: %v", i, err)
		}
		created = append(created, ids.UUID(contact.Id))
	}
	return created
}

// walkPreview reads every page of the preview as ctx and answers the ids in
// order. On each page it checks the count, and that truncated and next_cursor
// agree.
func walkPreview(ctx context.Context, t *testing.T, e *integration.Env, pageSize, wantCount int) []ids.UUID {
	t.Helper()
	handlers := filterPreviewHandlers{pool: e.Pool, collections: NewCollectionsStore(e.Pool)}
	engine, ok, err := handlers.collections.SegmentEngine(ctx, "contact")
	if err != nil || !ok {
		t.Fatalf("contact engine: ok=%v err=%v", ok, err)
	}
	var walked []ids.UUID
	var after *ids.UUID
	for page := 0; ; page++ {
		got, err := handlers.preview(ctx, engine, previewPagedFilter, after, pageSize, false)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if got.MatchCount != wantCount {
			t.Errorf("page %d match_count = %d, want %d on every page", page, got.MatchCount, wantCount)
		}
		if got.Truncated != (got.NextCursor != nil) {
			t.Fatalf("page %d truncated=%v with next_cursor %v; the two must agree", page, got.Truncated, got.NextCursor)
		}
		for _, row := range got.Rows {
			id, err := ids.Parse(fmt.Sprint(row["id"]))
			if err != nil {
				t.Fatalf("page %d row id %v: %v", page, row["id"], err)
			}
			walked = append(walked, id)
		}
		if got.NextCursor == nil {
			return walked
		}
		if after, err = storekit.DecodeIDCursor(*got.NextCursor); err != nil {
			t.Fatalf("page %d next_cursor %q: %v", page, *got.NextCursor, err)
		}
	}
}

func TestAFilterPreviewCursorVisitsEveryMatchOnceInIDOrder(t *testing.T) {
	e := integration.Setup(t)
	seeded := append(seedPagedContacts(t, e, e.Rep1, 4), seedPagedContacts(t, e, e.Rep2, 3)...)

	walked := walkPreview(e.Admin(), t, e, 2, len(seeded))
	if want := inIDOrder(seeded...); !slices.Equal(walked, want) {
		t.Fatalf("walked %v, want every match once in id order %v", walked, want)
	}
}

// Contacts are readable workspace-wide, so the narrowing a reader meets on them
// is capture privacy: a contact kept private to its owner. The update stands in
// for the capture writer, as the other capture-privacy suites here do.
func TestAFilterPreviewKeepsTheRowScopeOnEveryPage(t *testing.T) {
	e := integration.Setup(t)
	visible := seedPagedContacts(t, e, e.Rep1, 5)
	private := seedPagedContacts(t, e, e.Rep2, 5)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = ANY($1)`, private)

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})
	walked := walkPreview(reader, t, e, 2, len(visible))
	if want := inIDOrder(visible...); !slices.Equal(walked, want) {
		t.Fatalf("a reader walked %v, want only the rows they may see %v", walked, want)
	}
}
