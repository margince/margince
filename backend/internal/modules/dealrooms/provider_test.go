// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// The record seam's refusals. Each one must land before the store opens a
// transaction, so the provider here holds a store with no database at all.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ungrantedSeller is a signed-in seller who holds no deal room grant, so every
// store entry point refuses at its RBAC gate.
func ungrantedSeller() context.Context {
	user := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

func offlineProvider() *Provider {
	return &Provider{store: &Store{}}
}

// wantFieldRefusal asserts a 422 that names the field the caller must fix.
func wantFieldRefusal(t *testing.T, err error, field string) {
	t.Helper()
	var detailed *httperr.DetailedError
	if !errors.As(err, &detailed) || len(detailed.Fields) == 0 || detailed.Fields[0].Field != field {
		t.Fatalf("got %v, want a validation refusal naming %q", err, field)
	}
}

func TestEachRoomRecordPublishesTheFiltersItIsListedBy(t *testing.T) {
	p := offlineProvider()
	for entity, want := range map[datasource.EntityType][]string{
		datasource.EntityDealRoom:            {"deal_id", "include_archived", "participant_email", "sort", "state"},
		datasource.EntityDealRoomThread:      {"deal_room_id", "document_id"},
		datasource.EntityDealRoomDocument:    {"deal_room_id"},
		datasource.EntityDealRoomParticipant: {"active_only", "deal_room_id"},
		datasource.EntityDealRoomComment:     nil,
	} {
		if got := p.ListFilters(entity); !slices.Equal(got, want) {
			t.Errorf("%s lists by %v, want %v", entity, got, want)
		}
	}
}

func TestTheSeamRefusesARecordTheRoomDoesNotHold(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	var unsupported *datasource.UnsupportedEntityError
	if _, err := p.Read(ctx, datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}); !errors.As(err, &unsupported) {
		t.Errorf("reading a deal through the room's provider answered %v, want unsupported", err)
	}
	if _, _, _, err := p.SearchEntity(ctx, datasource.EntityDealRoomComment, nil, 10, nil, nil); !errors.As(err, &unsupported) {
		t.Errorf("listing comments answered %v, want unsupported: they are read inside their thread", err)
	}
	_, err := p.Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityDealRoomDocument,
		Fields:     map[string]any{"deal_room_id": ids.NewV7().String()},
		Source:     "agent",
	})
	if !errors.As(err, &unsupported) {
		t.Errorf("creating a document through the seam answered %v, want unsupported", err)
	}
}

func TestEveryRoomReadPassesTheRoomsReadGate(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	for _, entity := range []datasource.EntityType{
		datasource.EntityDealRoom, datasource.EntityDealRoomThread, datasource.EntityDealRoomDocument,
		datasource.EntityDealRoomParticipant, datasource.EntityDealRoomComment,
	} {
		if _, err := p.Read(ctx, datasource.EntityRef{Type: entity, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("reading a %s without the deal room grant answered %v, want permission denied", entity, err)
		}
	}
}

func TestRoomsHaveNoTextToSearch(t *testing.T) {
	text := "acme"
	_, _, _, err := offlineProvider().SearchEntity(ungrantedSeller(), datasource.EntityDealRoom, &text, 10, nil, nil)
	wantFieldRefusal(t, err, "q")
}

func TestARoomListBindsEveryFilterBeforeItsGate(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	all := map[string]string{
		"deal_id": ids.NewV7().String(), "state": "active", "sort": "-created_at",
		"include_archived": "true", "participant_email": "  Laura@Buyer.Example ",
	}
	if _, _, _, err := p.SearchEntity(ctx, datasource.EntityDealRoom, nil, 10, nil, all); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a well-formed room list answered %v, want the read gate's refusal", err)
	}
	var filterErr *storekit.FilterError
	if _, _, _, err := p.SearchEntity(ctx, datasource.EntityDealRoom, nil, 10, nil, map[string]string{"owner": "me"}); !errors.As(err, &filterErr) {
		t.Errorf("an unknown room filter answered %v, want a filter refusal", err)
	}
}

func TestARoomsRecordsAreListedOneRoomAtATime(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	room := ids.NewV7().String()
	for _, tc := range []struct {
		entity  datasource.EntityType
		filters map[string]string
	}{
		{datasource.EntityDealRoomThread, map[string]string{"deal_room_id": room, "document_id": ids.NewV7().String()}},
		{datasource.EntityDealRoomDocument, map[string]string{"deal_room_id": room}},
		{datasource.EntityDealRoomParticipant, map[string]string{"deal_room_id": room, "active_only": "true"}},
	} {
		if _, _, _, err := p.SearchEntity(ctx, tc.entity, nil, 10, nil, tc.filters); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("listing %s in one room answered %v, want the read gate's refusal", tc.entity, err)
		}
		_, _, _, err := p.SearchEntity(ctx, tc.entity, nil, 10, nil, nil)
		wantFieldRefusal(t, err, "deal_room_id")
	}
	var filterErr *storekit.FilterError
	if _, _, _, err := p.SearchEntity(ctx, datasource.EntityDealRoomDocument, nil, 10, nil, map[string]string{"document_id": room}); !errors.As(err, &filterErr) {
		t.Errorf("a document list narrowed by document_id answered %v, want a filter refusal", err)
	}
}

func TestASeamWriteNamesTheRoomAndThreadItGoesIn(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	create := func(entity datasource.EntityType, fields any) error {
		_, err := p.Create(ctx, datasource.CreateInput{EntityType: entity, Fields: fields, Source: "agent"})
		return err
	}
	wantFieldRefusal(t, create(datasource.EntityDealRoom, json.RawMessage(`[1]`)), "fields")
	wantFieldRefusal(t, create(datasource.EntityDealRoomThread, map[string]any{"body": "Terms?"}), "deal_room_id")
	wantFieldRefusal(t, create(datasource.EntityDealRoomThread, map[string]any{"deal_room_id": ids.Nil.String()}), "deal_room_id")
	wantFieldRefusal(t, create(datasource.EntityDealRoomComment, map[string]any{
		"deal_room_id": ids.NewV7().String(), "body": "Agreed.",
	}), "thread_id")
}

func TestASeamWriteDecodesStrictlyAndThenPassesTheWriteGate(t *testing.T) {
	p := offlineProvider()
	ctx := ungrantedSeller()
	room, thread := ids.NewV7().String(), ids.NewV7().String()
	for _, tc := range []struct {
		entity   datasource.EntityType
		fields   map[string]any
		required string
	}{
		{datasource.EntityDealRoom, map[string]any{"deal_id": ids.NewV7().String(), "title": "Acme rollout"}, "title"},
		{datasource.EntityDealRoomThread, map[string]any{"deal_room_id": room, "body": "Any questions?"}, "body"},
		{datasource.EntityDealRoomComment, map[string]any{"deal_room_id": room, "thread_id": thread, "body": "None."}, "body"},
	} {
		in := datasource.CreateInput{EntityType: tc.entity, Fields: tc.fields, Source: "agent"}
		if _, err := p.Create(ctx, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a well-formed %s answered %v, want the write gate's refusal", tc.entity, err)
		}

		tc.fields[tc.required] = 42
		var decodeErr *datasource.FieldDecodeError
		if _, err := p.Create(ctx, in); !errors.As(err, &decodeErr) {
			t.Errorf("a %s whose %s is a number answered %v, want a decode refusal", tc.entity, tc.required, err)
		}

		tc.fields[tc.required] = ""
		var fieldErr *fieldError
		if _, err := p.Create(ctx, in); !errors.As(err, &fieldErr) || fieldErr.field != tc.required {
			t.Errorf("a %s with a blank %s answered %v, want a refusal naming it", tc.entity, tc.required, err)
		}
	}
}
