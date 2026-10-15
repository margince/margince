// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"encoding/json"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Provider answers the record seam for the deal room and what it holds,
// through the store calls and request mapping the seller's REST routes run.
// Every read joins the room's deal under that deal's row scope.
type Provider struct {
	store *Store
}

// NewProvider builds the deal room's record provider over a workspace-bound
// handle.
func NewProvider(db *database.DB) *Provider {
	return &Provider{store: NewStore(db)}
}

const (
	filterDealID           = "deal_id"
	filterDealRoomID       = "deal_room_id"
	filterThreadID         = "thread_id"
	filterDocumentID       = "document_id"
	filterState            = "state"
	filterParticipantEmail = "participant_email"
	filterIncludeArchived  = "include_archived"
	filterSort             = "sort"
	filterActiveOnly       = "active_only"
)

var roomListFilters = storekit.FilterSet[ListRoomsInput]{
	filterDealID:          storekit.FilterID(func(in *ListRoomsInput, id *ids.DealID) { in.DealID = id }),
	filterState:           storekit.FilterWord(func(in *ListRoomsInput, v *string) { in.State = v }),
	filterSort:            storekit.FilterWord(func(in *ListRoomsInput, v *string) { in.Sort = v }),
	filterIncludeArchived: storekit.FilterFlag(func(in *ListRoomsInput, v *bool) { in.IncludeArchived = *v }),
	filterParticipantEmail: storekit.FilterWord(func(in *ListRoomsInput, v *string) {
		email := strings.ToLower(strings.TrimSpace(*v))
		in.ParticipantEmail = &email
	}),
}

// roomItems narrows a list of what one room holds.
type roomItems struct {
	roomID     *ids.DealRoomID
	documentID *ids.UUID
	activeOnly bool
}

var roomItemFilters = storekit.FilterSet[roomItems]{
	filterDealRoomID: storekit.FilterID(func(in *roomItems, id *ids.DealRoomID) { in.roomID = id }),
}

var threadListFilters = storekit.FilterSet[roomItems]{
	filterDealRoomID: storekit.FilterID(func(in *roomItems, id *ids.DealRoomID) { in.roomID = id }),
	filterDocumentID: storekit.FilterID(func(in *roomItems, id *ids.DealRoomDocumentID) {
		document := id.UUID
		in.documentID = &document
	}),
}

var participantListFilters = storekit.FilterSet[roomItems]{
	filterDealRoomID: storekit.FilterID(func(in *roomItems, id *ids.DealRoomID) { in.roomID = id }),
	filterActiveOnly: storekit.FilterFlag(func(in *roomItems, v *bool) { in.activeOnly = *v }),
}

// ListFilters names what each of the room's records is listed by.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	switch t {
	case datasource.EntityDealRoom:
		return roomListFilters.Names()
	case datasource.EntityDealRoomThread:
		return threadListFilters.Names()
	case datasource.EntityDealRoomDocument:
		return roomItemFilters.Names()
	case datasource.EntityDealRoomParticipant:
		return participantListFilters.Names()
	default:
		return nil
	}
}

// Read answers one room, or one thread with its replies: what a thread or a
// reply written through the seam is read back as.
func (p *Provider) Read(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	switch r.Type {
	case datasource.EntityDealRoom:
		v, err := p.store.GetRoom(ctx, ids.DealRoomID{UUID: r.ID})
		if err != nil {
			return datasource.Record{}, err
		}
		return datasource.NewRecord(r, v, v.Version)
	case datasource.EntityDealRoomThread:
		v, err := p.store.GetThread(ctx, r.ID)
		if err != nil {
			return datasource.Record{}, err
		}
		return datasource.NewRecord(r, v, v.Version)
	default:
		return datasource.Record{}, &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
}

// SearchEntity lists rooms, or one room's threads, documents or participants.
// None of them has text to search.
func (p *Provider) SearchEntity(ctx context.Context, t datasource.EntityType, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if text != nil {
		return nil, "", false, httperr.Validation("q", "unsupported", "deal rooms have no text to search: list them")
	}
	if t == datasource.EntityDealRoom {
		in := ListRoomsInput{Limit: &limit, Cursor: cursor}
		if err := roomListFilters.Apply(&in, filters); err != nil {
			return nil, "", false, err
		}
		rows, page, err := p.store.ListRooms(ctx, in)
		if err != nil {
			return nil, "", false, err
		}
		return recordsOf(t, rows, page, func(v crmcontracts.DealRoom) (ids.UUID, *int64) { return ids.UUID(v.Id), v.Version })
	}
	return p.searchRoomItems(ctx, t, filters)
}

func (p *Provider) searchRoomItems(ctx context.Context, t datasource.EntityType, filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	set, ok := map[datasource.EntityType]storekit.FilterSet[roomItems]{
		datasource.EntityDealRoomThread:      threadListFilters,
		datasource.EntityDealRoomDocument:    roomItemFilters,
		datasource.EntityDealRoomParticipant: participantListFilters,
	}[t]
	if !ok {
		return nil, "", false, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	var in roomItems
	if err := set.Apply(&in, filters); err != nil {
		return nil, "", false, err
	}
	if in.roomID == nil {
		return nil, "", false, httperr.Validation(filterDealRoomID, "required",
			"a room's records are listed one room at a time: name its deal_room_id")
	}
	switch t {
	case datasource.EntityDealRoomThread:
		rows, err := p.store.ListThreads(ctx, *in.roomID, in.documentID)
		if err != nil {
			return nil, "", false, err
		}
		return recordsOf(t, rows, storekit.Page{}, func(v crmcontracts.DealRoomThread) (ids.UUID, *int64) { return ids.UUID(v.Id), nil })
	case datasource.EntityDealRoomDocument:
		rows, page, err := p.store.ListDocuments(ctx, *in.roomID)
		if err != nil {
			return nil, "", false, err
		}
		return recordsOf(t, rows, page, func(v crmcontracts.DealRoomDocument) (ids.UUID, *int64) { return ids.UUID(v.Id), nil })
	default:
		rows, page, err := p.store.ListParticipants(ctx, *in.roomID, in.activeOnly)
		if err != nil {
			return nil, "", false, err
		}
		return recordsOf(t, rows, page, func(v crmcontracts.DealRoomParticipant) (ids.UUID, *int64) { return ids.UUID(v.Id), nil })
	}
}

func recordsOf[V any](t datasource.EntityType, rows []V, page storekit.Page, key func(V) (ids.UUID, *int64)) ([]datasource.Record, string, bool, error) {
	records := make([]datasource.Record, 0, len(rows))
	for _, v := range rows {
		id, version := key(v)
		rec, err := datasource.NewRecord(datasource.EntityRef{Type: t, ID: id}, v, version)
		if err != nil {
			return nil, "", false, err
		}
		records = append(records, rec)
	}
	return records, page.NextCursor, page.HasMore, nil
}

// Create opens a room, a thread in one, or a reply in a thread. It writes as
// the seller's side, under the rules the seller's REST routes apply.
func (p *Provider) Create(ctx context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	raw, err := datasource.RawFields(in.Fields)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	fields, roomID, err := takeRoomIDs(raw, in.EntityType != datasource.EntityDealRoom)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	if fields["source"], err = json.Marshal(in.Source); err != nil {
		return datasource.EntityRef{}, err
	}
	switch in.EntityType {
	case datasource.EntityDealRoom:
		return p.createRoom(ctx, fields)
	case datasource.EntityDealRoomThread:
		return p.openThread(ctx, roomID, fields)
	case datasource.EntityDealRoomComment:
		return p.reply(ctx, roomID, fields)
	default:
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.EntityType)}
	}
}

// takeRoomIDs lifts the room a thread or reply goes in out of its fields. The
// route names it in its path, the seam among the fields.
func takeRoomIDs(raw json.RawMessage, needsRoom bool) (map[string]json.RawMessage, ids.DealRoomID, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, ids.DealRoomID{}, httperr.Validation("fields", "malformed_json", err.Error())
	}
	if !needsRoom {
		return fields, ids.DealRoomID{}, nil
	}
	var room ids.UUID
	if err := json.Unmarshal(fields[filterDealRoomID], &room); err != nil || room.IsZero() {
		return nil, ids.DealRoomID{}, httperr.Validation(filterDealRoomID, "required", "name the room by its deal_room_id")
	}
	delete(fields, filterDealRoomID)
	return fields, ids.DealRoomID{UUID: room}, nil
}

func (p *Provider) createRoom(ctx context.Context, fields map[string]json.RawMessage) (datasource.EntityRef, error) {
	var req crmcontracts.CreateDealRoomRequest
	if err := decodeFields(fields, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	in, err := createInput(req)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	room, err := p.store.CreateRoom(ctx, in)
	return datasource.EntityRef{Type: datasource.EntityDealRoom, ID: ids.UUID(room.Id)}, err
}

func (p *Provider) openThread(ctx context.Context, roomID ids.DealRoomID, fields map[string]json.RawMessage) (datasource.EntityRef, error) {
	var req crmcontracts.OpenDealRoomThreadRequest
	if err := decodeFields(fields, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	in, err := openThreadInput(req, true)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	thread, err := p.store.OpenThread(ctx, roomID, in)
	return datasource.EntityRef{Type: datasource.EntityDealRoomThread, ID: ids.UUID(thread.Id)}, err
}

func (p *Provider) reply(ctx context.Context, roomID ids.DealRoomID, fields map[string]json.RawMessage) (datasource.EntityRef, error) {
	var thread ids.UUID
	if err := json.Unmarshal(fields[filterThreadID], &thread); err != nil || thread.IsZero() {
		return datasource.EntityRef{}, httperr.Validation(filterThreadID, "required", "a reply names the thread it answers: its thread_id")
	}
	delete(fields, filterThreadID)
	var req crmcontracts.PostDealRoomCommentRequest
	if err := decodeFields(fields, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	body, source, err := commentInput(req, true)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	answered, err := p.store.Reply(ctx, roomID, thread, body, source, optionalUUID(req.RequestId))
	return datasource.EntityRef{Type: datasource.EntityDealRoomThread, ID: ids.UUID(answered.Id)}, err
}

func decodeFields[T any](fields map[string]json.RawMessage, into *T) error {
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return datasource.StrictDecode(body, into)
}
