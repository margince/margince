// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Every reader that lists mail shows a mail two mailboxes captured once: the
// thread drawer, search and the context graph, as the timeline does. A reader
// of both copies is shown their own, and a reader of one copy that copy.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

func TestEveryMailReaderShowsBothCopiesAsTheReadersOwn(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
	s.first(t)
	s.second(t)
	fileCopiesUnder(t, s.e, "contact", s.contact.UUID)
	rows := capturedMailRows(t, s.e)
	firstCopy, secondCopy := rows[0].ID, rows[1].ID
	for reader, own := range map[ids.UUID]ids.UUID{s.e.Rep1: firstCopy, s.e.Rep3: secondCopy} {
		assertEveryReaderShowsOnly(t, s, reader, own, firstCopy, secondCopy)
	}
}

func TestEveryMailReaderShowsAOneCopyReaderThatCopy(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: false})
	s.first(t)
	s.second(t)
	rows := capturedMailRows(t, s.e)
	fileCopyUnder(t, s, s.e.Rep1, rows[0].ID, "contact", s.contact.UUID)
	fileCopyUnder(t, s, s.e.Rep3, rows[1].ID, "contact", s.contact.UUID)
	assertEveryReaderShowsOnly(t, s, s.e.Rep1, rows[0].ID, rows[0].ID)
	assertEveryReaderShowsOnly(t, s, s.e.Rep3, rows[1].ID, rows[0].ID, rows[1].ID)
}

// fileCopyUnder files one activity under a record as the seat that may open
// it: the other seat cannot reach a copy only its owner reads.
func fileCopyUnder(t *testing.T, s seatCopyEnv, seat, copyID ids.UUID, entityType string, entity ids.UUID) {
	t.Helper()
	ctx := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), s.e.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true, Update: true}, "activity": {Read: true, Update: true}, "deal": {Read: true, Update: true},
			}, RowScope: principal.RowScopeAll},
		})
	if _, err := activities.NewStore(s.e.DB()).RelinkActivity(ctx, ids.From[ids.ActivityKind](copyID),
		activities.RelinkActivityInput{EntityType: entityType, EntityID: entity}); err != nil {
		t.Fatalf("filing %s under the %s as %s: %v", copyID, entityType, seat, err)
	}
}

// assertEveryReaderShowsOnly checks that each mail reader shows reader the one
// mail as want, and that opening any copy the reader may open draws it once.
func assertEveryReaderShowsOnly(t *testing.T, s seatCopyEnv, reader, want ids.UUID, readable ...ids.UUID) {
	t.Helper()
	ctx := readerCtx(s.e, reader)
	page, err := s.e.Store.Search(ctx, search.Input{Query: "Telematics", Types: []string{"activity"}, Limit: 10})
	if err != nil {
		t.Fatalf("searching as %s: %v", reader, err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != want {
		t.Errorf("search as %s: got %+v, want the one hit %s", reader, page.Hits, want)
	}
	if got := recentTouches(t, s, reader); len(got) != 1 || got[0] != want {
		t.Errorf("context graph as %s: recent touches %v, want only %s", reader, got, want)
	}
	for _, opened := range readable {
		if got := drawnMessages(t, s, reader, opened); got != 1 {
			t.Errorf("drawer as %s on copy %s draws %d messages, want 1", reader, opened, got)
		}
	}
}

// recentTouches is the context graph's touches around the sender, as the
// assistant reads them.
func recentTouches(t *testing.T, s seatCopyEnv, reader ids.UUID) []ids.UUID {
	t.Helper()
	picture, err := search.NewRetriever(s.e.Store, nil).AssembleContext(readerCtx(s.e, reader),
		datasource.EntityRef{Type: datasource.EntityContact, ID: s.contact.UUID}, retrieval.AssembleOptions{MaxItems: 10})
	if err != nil {
		t.Fatalf("assembling the sender's context as %s: %v", reader, err)
	}
	var touches []ids.UUID
	for _, section := range picture.Sections {
		if section.Name == "recent_touches" {
			for _, item := range section.Items {
				touches = append(touches, item.Ref.ID)
			}
		}
	}
	return touches
}

// drawnMessages counts what the email drawer shows: the opened message and the
// thread members that are not it.
func drawnMessages(t *testing.T, s seatCopyEnv, reader, opened ids.UUID) int {
	t.Helper()
	shown, err := activities.NewStore(s.e.DB()).GetEmailPresentation(readerCtx(s.e, reader), ids.From[ids.ActivityKind](opened), nil)
	if err != nil {
		t.Fatalf("opening %s as %s: %v", opened, reader, err)
	}
	if shown.Thread == nil {
		t.Fatalf("opening %s as %s: the captured mail carries no thread", opened, reader)
	}
	drawn := 1
	for _, member := range shown.Thread.Members {
		if ids.UUID(member.ActivityId) != opened {
			drawn++
		}
	}
	return drawn
}
