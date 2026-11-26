// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Paging a filter preview. The cursor walks every match once in id order, and
// each page carries the full count. A reader meets no row they may not see.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
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
	var token string
	for page := 0; ; page++ {
		scope, after, err := previewCursor("contact", previewPagedFilter, token)
		if err != nil {
			t.Fatalf("page %d cursor %q: %v", page, token, err)
		}
		got, err := handlers.preview(ctx, engine, previewPagedFilter, scope, after, pageSize, false)
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
		token = *got.NextCursor
	}
}

// A preview cursor continues only the filter that minted it. One from another
// filter or another record type is refused, not read as a place to resume.
func TestAFilterPreviewCursorContinuesOnlyItsOwnFilter(t *testing.T) {
	e := integration.Setup(t)
	seedPagedContacts(t, e, e.Rep1, 3)
	handlers := filterPreviewHandlers{pool: e.Pool, collections: NewCollectionsStore(e.Pool)}
	engine, ok, err := handlers.collections.SegmentEngine(e.Admin(), "contact")
	if err != nil || !ok {
		t.Fatalf("contact engine: ok=%v err=%v", ok, err)
	}
	scope, _, err := previewCursor("contact", previewPagedFilter, "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := handlers.preview(e.Admin(), engine, previewPagedFilter, scope, nil, 1, false)
	if err != nil || first.NextCursor == nil {
		t.Fatalf("the first page answered %+v, %v; want a cursor", first, err)
	}
	other := storekit.Predicate{Field: "name", Op: "contains", Value: "Another Subject"}
	var malformed *storekit.MalformedCursorError
	for name, attempt := range map[string]func() error{
		"another filter": func() error { _, _, err := previewCursor("contact", other, *first.NextCursor); return err },
		"another type":   func() error { _, _, err := previewCursor("company", previewPagedFilter, *first.NextCursor); return err },
	} {
		if err := attempt(); !errors.As(err, &malformed) {
			t.Errorf("a cursor read under %s gave %v, want a malformed cursor", name, err)
		}
	}
	if _, after, err := previewCursor("contact", previewPagedFilter, *first.NextCursor); err != nil || after == nil {
		t.Fatalf("the minting filter refused its own cursor: %v", err)
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

// The count, the page and the rows read one snapshot. The row read waits on a
// lock over contact_email, which only it touches, while a rename commits. A row
// read in its own snapshot would print the new name.
func TestAFilterPreviewReadsItsRowsInTheSnapshotOfItsPage(t *testing.T) {
	e := integration.Setup(t)
	target := seedPagedContacts(t, e, e.Rep1, 1)[0]
	handlers := filterPreviewHandlers{pool: e.Pool, collections: NewCollectionsStore(e.Pool)}
	engine, ok, err := handlers.collections.SegmentEngine(e.Admin(), "contact")
	if err != nil || !ok {
		t.Fatalf("contact engine: ok=%v err=%v", ok, err)
	}

	holder := integration.FreshOwnerConn(t)
	locked, err := holder.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := locked.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("release the contact_email lock: %v", err)
		}
	})
	if _, err := locked.Exec(context.Background(), "LOCK TABLE contact_email IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	var holderPID int32
	if err := locked.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&holderPID); err != nil {
		t.Fatal(err)
	}

	type answer struct {
		preview crmcontracts.FilterPreview
		err     error
	}
	scope, _, err := previewCursor("contact", previewPagedFilter, "")
	if err != nil {
		t.Fatal(err)
	}
	answered, returned := make(chan answer, 1), make(chan struct{})
	go func() {
		defer close(returned)
		got, err := handlers.preview(e.Admin(), engine, previewPagedFilter, scope, nil, 10, false)
		answered <- answer{got, err}
	}()
	observer := integration.OwnerConn(t)
	testdb.AwaitQueuedBehind(t, observer, holderPID, 1, returned, "the preview reading contact_email")
	e.WsExec(t, `UPDATE contact SET full_name = 'Renamed meanwhile' WHERE id = $1`, target)
	if err := locked.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := <-answered
	if got.err != nil {
		t.Fatal(got.err)
	}
	if len(got.preview.Rows) != 1 || got.preview.Rows[0]["full_name"] == "Renamed meanwhile" {
		t.Fatalf("rows = %v, want the one match as the page's snapshot saw it", got.preview.Rows)
	}
}
