// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal room through the record seam an agent reaches it by. It runs the
// same store, mapping and deal scope as the seller's REST routes.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// roomWriter is the bootstrap admin on the seam. It must be a real app_user,
// because a thread's author is a foreign key into that table.
func roomWriter(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	admin := readerByEmail(t, e, "ada@example.com")
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + admin.String(), UserID: admin,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":      {Read: true, Update: true},
				"deal_room": {Create: true, Read: true, Update: true},
				// A room file that arrived on an email shows only to a seat that may read the email.
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func seamCreate(ctx context.Context, t *testing.T, seam *dealrooms.Provider, entity datasource.EntityType, fields map[string]any) datasource.EntityRef {
	t.Helper()
	ref, err := seam.Create(ctx, datasource.CreateInput{EntityType: entity, Fields: fields, Source: "agent"})
	if err != nil || ref.ID.IsZero() {
		t.Fatalf("creating a %s on the seam answered %+v (%v)", entity, ref, err)
	}
	return ref
}

func seamList(ctx context.Context, t *testing.T, seam *dealrooms.Provider, entity datasource.EntityType, filters map[string]string) []ids.UUID {
	t.Helper()
	records, _, _, err := seam.SearchEntity(ctx, entity, nil, 25, nil, filters)
	if err != nil {
		t.Fatalf("listing %s by %v: %v", entity, filters, err)
	}
	listed := make([]ids.UUID, 0, len(records))
	for _, r := range records {
		listed = append(listed, r.Ref.ID)
	}
	return listed
}

func listsOnly(listed []ids.UUID, want ids.UUID) bool {
	return len(listed) == 1 && listed[0] == want
}

// An agent opens a room, speaks in it and is answered. Every step reads back
// on the seam as the seller's routes would show it.
func TestAnAgentOpensARoomAndHoldsItsConversationOnTheSeam(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dealID := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))
	seam := dealrooms.NewProvider(e.DB())
	w := roomWriter(t, e)

	room := seamCreate(w, t, seam, datasource.EntityDealRoom, map[string]any{"deal_id": dealID, "title": "Acme rollout"})
	read, err := seam.Read(w, room)
	if err != nil || read.Version == 0 {
		t.Fatalf("reading the new room answered version %d (%v)", read.Version, err)
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.ID.String()+"/participants", AnyMap{
		"full_name": "Laura Buyer", "email": "laura@buyer.example", "capability": "comment", "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("inviting the buyer = %d", status)
	}

	inRoom := map[string]string{"deal_room_id": room.ID.String()}
	thread := seamCreate(w, t, seam, datasource.EntityDealRoomThread, map[string]any{
		"deal_room_id": room.ID.String(), "body": "Any questions on the terms?",
	})
	answered := seamCreate(w, t, seam, datasource.EntityDealRoomComment, map[string]any{
		"deal_room_id": room.ID.String(), "thread_id": thread.ID.String(), "body": "Clause 4 is settled.",
	})
	if answered != thread {
		t.Errorf("a reply reads back as %+v, want the thread it answers %+v", answered, thread)
	}
	record, err := seam.Read(w, thread)
	if err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	var got struct {
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(record.Fields, &got); err != nil {
		t.Fatalf("decoding the thread: %v", err)
	}
	if len(got.Comments) != 2 || got.Comments[1].Body != "Clause 4 is settled." {
		t.Errorf("the thread reads back as %+v, want the reply after the question", got)
	}

	if listed := seamList(w, t, seam, datasource.EntityDealRoomThread, inRoom); !listsOnly(listed, thread.ID) {
		t.Errorf("the room's threads list as %v, want only %s", listed, thread.ID)
	}
	if listed := seamList(w, t, seam, datasource.EntityDealRoomParticipant, map[string]string{
		"deal_room_id": room.ID.String(), "active_only": "true",
	}); len(listed) != 1 {
		t.Errorf("the room's active participants list as %v, want the one buyer invited", listed)
	}
	for _, filters := range []map[string]string{
		{"deal_id": dealID},
		{"participant_email": "  LAURA@Buyer.Example "},
	} {
		if listed := seamList(w, t, seam, datasource.EntityDealRoom, filters); !listsOnly(listed, room.ID) {
			t.Errorf("rooms listed by %v answered %v, want only %s", filters, listed, room.ID)
		}
	}
	for _, filters := range []map[string]string{
		{"deal_id": ids.NewV7().String()},
		{"participant_email": "someone@else.example"},
	} {
		if listed := seamList(w, t, seam, datasource.EntityDealRoom, filters); len(listed) != 0 {
			t.Errorf("rooms listed by %v answered %v, want none: the filter must narrow", filters, listed)
		}
	}
	if _, err := seam.Read(w, datasource.EntityRef{Type: datasource.EntityDealRoomThread, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a thread no room holds answered %v, want not found", err)
	}
}

// A room's documents list and read on the seam, and a thread opened on one
// is what the document_id filter narrows the room's threads to.
func TestARoomsDocumentsAndTheirThreadsReadOnTheSeam(t *testing.T) {
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	_, attachmentID := captureFileOnEmail(t, e, blob, room.dealID)
	var doc struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/documents", AnyMap{
		"attachment_id": attachmentID, "group_key": "legal", "source": "manual",
	}, nil, &doc); status != http.StatusCreated {
		t.Fatalf("adding a file to the room = %d", status)
	}
	seam := dealrooms.NewProvider(e.DB())
	w := roomWriter(t, e)
	docID := ids.MustParse(doc.ID)

	inRoom := map[string]string{"deal_room_id": room.roomID}
	if listed := seamList(w, t, seam, datasource.EntityDealRoomDocument, inRoom); !listsOnly(listed, docID) {
		t.Errorf("the room's documents list as %v, want only %s", listed, docID)
	}
	docRef := datasource.EntityRef{Type: datasource.EntityDealRoomDocument, ID: docID}
	if record, err := seam.Read(w, docRef); err != nil || record.Version == 0 {
		t.Errorf("reading the document answered version %d (%v)", record.Version, err)
	}
	if _, err := seam.Read(w, datasource.EntityRef{Type: datasource.EntityDealRoomDocument, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a document no room holds answered %v, want not found", err)
	}

	seamCreate(w, t, seam, datasource.EntityDealRoomThread, map[string]any{
		"deal_room_id": room.roomID, "body": "Welcome to the room.",
	})
	onDocument := seamCreate(w, t, seam, datasource.EntityDealRoomThread, map[string]any{
		"deal_room_id": room.roomID, "document_id": doc.ID, "body": "Please change clause 4.", "required_change": true,
	})
	if record, err := seam.Read(w, onDocument); err != nil || !strings.Contains(string(record.Fields), `"required_change":true`) {
		t.Errorf("the document's thread reads back as %s (%v), want a required change", record.Fields, err)
	}
	if listed := seamList(w, t, seam, datasource.EntityDealRoomThread, inRoom); len(listed) != 2 {
		t.Errorf("the room's threads list as %v, want both", listed)
	}
	if listed := seamList(w, t, seam, datasource.EntityDealRoomThread, map[string]string{
		"deal_room_id": room.roomID, "document_id": doc.ID,
	}); !listsOnly(listed, onDocument.ID) {
		t.Errorf("the document's threads list as %v, want only %s", listed, onDocument.ID)
	}
}
