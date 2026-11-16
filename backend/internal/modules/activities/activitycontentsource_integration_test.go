// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnceDecisionActivityKeepsAudienceAndArchiveReads(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	written := loggedNote(ctx, t, e)
	id := ids.From[ids.ActivityKind](ids.UUID(written.Id))
	if _, err := store.SetAudience(ctx, id, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	colleague := principal.WithActor(ctx, actor)
	got, err := store.GetActivity(colleague, id, storekit.LiveOnly)
	if err != nil || got.ContentState == nil || *got.ContentState != "withheld" || got.Body != nil || got.Subject != nil {
		t.Fatalf("colleague read = %+v, %v", got, err)
	}
	own, err := store.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil || own.Body == nil || *own.Body != noteBody {
		t.Fatalf("author read = %+v, %v", own, err)
	}
	if _, err := store.ArchiveActivity(e.asArchiver(), id, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetActivity(ctx, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("archived live read = %v", err)
	}
	archived, err := store.GetActivity(ctx, id, storekit.IncludeArchived)
	if err != nil || archived.ArchivedAt == nil || archived.ContentState == nil || *archived.ContentState != "available" || archived.Body == nil || *archived.Body != noteBody {
		t.Fatalf("archived read = %+v, %v", archived, err)
	}
}

func TestOnceDecisionEmailSummaryKeepsContentAdmission(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	written, _, err := store.LogActivity(ctx, LogActivityInput{
		Kind: "email", Subject: new("Price request"), Body: new("Please send pricing."),
		Direction: new("inbound"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(written.Id)
	summaries, err := store.EmailSummariesByIDAt(ctx, []ids.UUID{id}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	summary, ok := summaries[id]
	if !ok || summary.Subject == nil || *summary.Subject != "Price request" || summary.Preview == nil || *summary.Preview != "Please send pricing." {
		t.Fatalf("readable summary lost its content: %+v", summary)
	}
	if _, err := store.SetAudience(ctx, ids.From[ids.ActivityKind](id), SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	hidden, err := store.EmailSummariesByIDAt(principal.WithActor(ctx, actor), []ids.UUID{id}, epoch)
	if err != nil || len(hidden) != 0 {
		t.Fatalf("withheld email admitted to content batch: %d, %v", len(hidden), err)
	}
}

func TestOnceDecisionActivityPagesKeepCursorAndCount(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	want := map[ids.UUID]string{}
	for _, body := range []string{"First body", "Second body"} {
		row, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new("Cursor"), Body: &body, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		want[ids.UUID(row.Id)] = body
	}
	for _, collapsed := range []bool{false, true} {
		in := ListActivitiesInput{Limit: new(1), CollapseSeatCopies: collapsed}
		count, err := store.CountActivities(ctx, in)
		if err != nil || count != len(want) {
			t.Fatalf("count = %d, %v", count, err)
		}
		seen := map[ids.UUID]bool{}
		for pageNumber := range 2 {
			rows, page, err := store.ListActivities(ctx, in)
			if err != nil || len(rows) != 1 {
				t.Fatalf("page %d length = %d, %v", pageNumber, len(rows), err)
			}
			row := rows[0]
			id := ids.UUID(row.Id)
			if seen[id] || row.Body == nil || *row.Body != want[id] {
				t.Fatal("cursor repeated a row or content was transposed")
			}
			seen[id] = true
			if page.HasMore != (pageNumber == 0) {
				t.Fatalf("page %d has_more = %t", pageNumber, page.HasMore)
			}
			in.Cursor = &page.NextCursor
		}
	}
}
