// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// roomSeats are the grants a seat needs to open and find rooms: the deal it
// hangs off and the room itself.
func roomSeats(read bool) map[string]principal.ObjectGrant {
	grants := searchReadGrants()
	grants["deal"] = principal.ObjectGrant{Read: true, Update: true}
	if read {
		grants["deal_room"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	}
	return grants
}

func (e *SearchEnv) asSeat(user, team ids.UUID, scope principal.RowScope, objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{team}, Permissions: principal.Permissions{Objects: objects, RowScope: scope},
	})
}

type roomFixture struct {
	first, second, archived ids.UUID
}

// seedRooms opens three rooms through the real store: two live rooms on deals
// owned by different teams, and one that is archived. All three titles share the
// word "harbour".
func seedRooms(t *testing.T, e *SearchEnv) roomFixture {
	t.Helper()
	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Room pipeline')`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
	                      VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := func(owner ids.UUID, name string) ids.UUID {
		return e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		                    VALUES ($1, $2, $3, $4, $5, 'manual', 'human:x')`, owner, name, pipeline, stage)
	}
	writer := e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, roomSeats(true))
	store := dealrooms.NewStore(e.DB())
	open := func(dealID ids.UUID, title string, welcome *string) ids.UUID {
		room, err := store.CreateRoom(writer, dealrooms.CreateRoomInput{
			DealID: ids.From[ids.DealKind](dealID), Title: title, WelcomeMessage: welcome, Source: "manual",
		})
		if err != nil {
			t.Fatalf("opening room %q: %v", title, err)
		}
		return ids.UUID(room.Id)
	}
	welcome := "Quarterly pricing walkthrough"
	f := roomFixture{
		first:  open(deal(e.Rep1, "Harbour deal one"), "Harbour renewal room", &welcome),
		second: open(deal(e.Rep3, "Harbour deal two"), "Harbour second room", nil),
	}
	f.archived = open(deal(e.Rep1, "Harbour deal three"), "Harbour closed room", nil)
	if _, err := store.ArchiveRoom(writer, ids.From[ids.DealRoomKind](f.archived), nil); err != nil {
		t.Fatalf("archiving the third room: %v", err)
	}
	return f
}

func sortIDs(found []ids.UUID) {
	slices.SortFunc(found, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
}

func roomHitIDs(ctx context.Context, t *testing.T, e *SearchEnv, query string) []ids.UUID {
	t.Helper()
	page, err := e.Store.Search(ctx, search.Input{Query: query, Types: []string{"deal_room"}})
	if err != nil {
		t.Fatalf("searching %q: %v", query, err)
	}
	var found []ids.UUID
	for _, hit := range page.Hits {
		found = append(found, hit.ID)
	}
	return found
}

func searchEnvRoomList(ctx context.Context, t *testing.T, e *SearchEnv) []ids.UUID {
	t.Helper()
	rooms, _, err := dealrooms.NewStore(e.DB()).ListRooms(ctx, dealrooms.ListRoomsInput{})
	if err != nil {
		t.Fatal(err)
	}
	var listed []ids.UUID
	for _, room := range rooms {
		listed = append(listed, ids.UUID(room.Id))
	}
	return listed
}

// A seat finds the rooms the room list shows it. A human seat's deal scope
// admits every deal, so either team finds both live rooms and nobody finds the
// archived one. The test compares with the list itself and not a fixed set.
func TestASeatFindsTheDealRoomsTheRoomListShowsIt(t *testing.T) {
	e := SetupSearch(t)
	f := seedRooms(t, e)

	for name, ctx := range map[string]context.Context{
		"team 1": e.asSeat(e.Rep1, e.Team1, principal.RowScopeTeam, roomSeats(true)),
		"team 2": e.asSeat(e.Rep3, e.Team2, principal.RowScopeTeam, roomSeats(true)),
		"all":    e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, roomSeats(true)),
		"own":    e.asSeat(e.Rep3, e.Team2, principal.RowScopeOwn, roomSeats(true)),
	} {
		got := roomHitIDs(ctx, t, e, "harbour")
		sortIDs(got)
		want := []ids.UUID{f.first, f.second}
		sortIDs(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: search found %v, want the two live rooms %v", name, got, want)
		}
		listed := searchEnvRoomList(ctx, t, e)
		sortIDs(listed)
		if !slices.Equal(got, listed) {
			t.Errorf("%s: search found %v but the room list shows %v", name, got, listed)
		}
		if byMessage := roomHitIDs(ctx, t, e, "walkthrough"); !slices.Equal(byMessage, []ids.UUID{f.first}) {
			t.Errorf("%s: searching the welcome message found %v, want the one room", name, byMessage)
		}
	}
}

// A seat that may see the deal but holds no deal_room read finds no room, and
// is not told one exists.
func TestASeatWithoutRoomReadFindsNoDealRoom(t *testing.T) {
	e := SetupSearch(t)
	seedRooms(t, e)
	ctx := e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, roomSeats(false))
	if got := roomHitIDs(ctx, t, e, "harbour"); len(got) != 0 {
		t.Errorf("a seat without deal_room read found rooms %v", got)
	}
}

// A buyer has no search by design. Even with every grant and row scope all, a
// buyer gets the page a query that matches nothing gets. The answer cannot say
// that a room exists.
func TestABuyerSearchingForARoomGetsTheAnswerOfAQueryThatMatchesNothing(t *testing.T) {
	e := SetupSearch(t)
	seedRooms(t, e)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalBuyer, ID: "buyer:" + ids.NewV7().String(),
		Permissions: principal.Permissions{Objects: roomSeats(true), RowScope: principal.RowScopeAll},
	})

	hit, hitErr := e.Store.Search(ctx, search.Input{Query: "harbour"})
	miss, missErr := e.Store.Search(ctx, search.Input{Query: "zzqxnothing"})
	if hitErr != nil || missErr != nil {
		t.Fatalf("a buyer's search answers %v for a title and %v for a miss, want no error for both", hitErr, missErr)
	}
	if len(hit.Hits) != 0 || len(miss.Hits) != 0 {
		t.Errorf("a buyer found %+v (miss: %+v), want nothing", hit.Hits, miss.Hits)
	}
}

// A room hit names its deal, the record a client opens to reach the room.
func TestARoomHitNamesItsDeal(t *testing.T) {
	e := SetupSearch(t)
	f := seedRooms(t, e)
	ctx := e.asSeat(e.Rep1, e.Team1, principal.RowScopeTeam, roomSeats(true))
	page, err := e.Store.Search(ctx, search.Input{Query: "renewal", Types: []string{"deal_room"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != f.first {
		t.Fatalf("searching the room title found %+v, want the first room", page.Hits)
	}
	var want ids.UUID
	if err := e.Owner.QueryRow(context.Background(), `SELECT deal_id FROM deal_room WHERE id = $1`, f.first).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if page.Hits[0].DealID == nil || *page.Hits[0].DealID != want {
		t.Errorf("the room hit names deal %v, want %v", page.Hits[0].DealID, want)
	}
}

// per_type at its contract maximum, with every type matching, still answers and
// stays within the largest ranked page.
func TestAGroupedSearchAtThePerTypeCapAnswersWithinTheRankedPageLimit(t *testing.T) {
	e := SetupSearch(t)
	seedRooms(t, e)
	perType := 20
	page, err := e.Store.Search(e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, roomSeats(true)),
		search.Input{Query: "harbour", PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	if !hasType(page.Hits, "deal_room") || !hasType(page.Hits, "deal") {
		t.Errorf("a grouped page answered %+v, want rooms and deals", page.Hits)
	}
	unbounded := math.MaxInt
	if largest := storekit.ClampLimit(&unbounded); len(page.Hits) > largest {
		t.Errorf("a grouped page holds %d hits, past the ranked page limit of %d", len(page.Hits), largest)
	}
}

// A seat that reads rooms but not deals sees no room through any door.
// A room is seen through its deal, so the deal grant comes first.
func TestASeatWithoutDealReadFindsNoRoomAndListsNone(t *testing.T) {
	e := SetupSearch(t)
	seedRooms(t, e)
	grants := roomSeats(true)
	delete(grants, "deal")
	ctx := e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, grants)

	if got := roomHitIDs(ctx, t, e, "harbour"); len(got) != 0 {
		t.Errorf("a seat without deal read found rooms %v", got)
	}
	perType := 5
	grouped, err := e.Store.Search(ctx, search.Input{Query: "harbour", PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	if hasType(grouped.Hits, "deal_room") || slices.Contains(grouped.TypesWithMore, "deal_room") {
		t.Errorf("a grouped page for a seat without deal read names rooms: hits %+v, more %v", grouped.Hits, grouped.TypesWithMore)
	}
	rooms, _, err := dealrooms.NewStore(e.DB()).ListRooms(ctx, dealrooms.ListRoomsInput{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(rooms) != 0 {
		t.Errorf("the room list for a seat without deal read answered %d rooms and %v, want a refusal", len(rooms), err)
	}
}

// The room of an archived deal and the room of a closed deal are both listed.
// The deal scope asks who may see the deal, not what state it is in.
// Search shows the same rooms the list does.
func TestARoomOfAnArchivedOrClosedDealIsFoundWhenTheListShowsIt(t *testing.T) {
	e := SetupSearch(t)
	f := seedRooms(t, e)
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE name = 'Harbour deal one'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE deal SET status = 'won', closed_at = now() WHERE name = 'Harbour deal two'`); err != nil {
		t.Fatal(err)
	}
	ctx := e.asSeat(e.Rep1, e.Team1, principal.RowScopeTeam, roomSeats(true))

	found := roomHitIDs(ctx, t, e, "harbour")
	listed := searchEnvRoomList(ctx, t, e)
	sortIDs(found)
	sortIDs(listed)
	if !slices.Equal(found, listed) {
		t.Fatalf("search found %v but the room list shows %v", found, listed)
	}
	for _, room := range []ids.UUID{f.first, f.second} {
		if !slices.Contains(found, room) {
			t.Errorf("room %v of an archived or closed deal is missing from %v", room, found)
		}
	}
}

// A seat that reads rooms but not deals gets one answer for a room part that
// exists and one that does not. So the status cannot say which ids are real.
func TestARoomPartReadWithoutDealReadAnswersTheSameForAMissingAndAnExistingId(t *testing.T) {
	e := SetupSearch(t)
	f := seedRooms(t, e)
	thread := e.SeedID(t, `INSERT INTO deal_room_thread (id, room_id, author_user_id, source, captured_by)
	                       VALUES ($1, $2, $3, 'manual', 'human:x')`, f.first, e.Rep1)
	comment := e.SeedID(t, `INSERT INTO deal_room_comment (id, room_id, thread_id, author_user_id, body, source, captured_by)
	                        VALUES ($1, $2, $3, $4, 'Looks good', 'manual', 'human:x')`, f.first, thread, e.Rep1)
	participant := e.SeedID(t, `INSERT INTO deal_room_participant (id, room_id, email, full_name, source, captured_by)
	                            VALUES ($1, $2, 'buyer@room.test', 'Buyer', 'manual', 'human:x')`, f.first)
	attachment := e.SeedID(t, `INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
	                           VALUES ($1, 'deal', (SELECT deal_id FROM deal_room WHERE id=$2), 'terms.pdf', 'k/terms', 'manual', 'human:x')`, f.first)
	document := e.SeedID(t, `INSERT INTO deal_room_document (id, room_id, attachment_id, group_key, title, source, captured_by)
	                         VALUES ($1, $2, $3, 'commercial', 'Terms', 'manual', 'human:x')`, f.first, attachment)

	grants := roomSeats(true)
	delete(grants, "deal")
	ctx := e.asSeat(e.Rep1, e.Team1, principal.RowScopeAll, grants)
	store := dealrooms.NewStore(e.DB())
	missing := ids.NewV7()
	reads := map[string]func(id ids.UUID) error{
		"thread":      func(id ids.UUID) error { _, err := store.GetThread(ctx, id); return err },
		"document":    func(id ids.UUID) error { _, err := store.GetDocument(ctx, id); return err },
		"participant": func(id ids.UUID) error { _, err := store.GetParticipant(ctx, id); return err },
		"comment":     func(id ids.UUID) error { _, err := store.GetComment(ctx, id); return err },
	}
	existing := map[string]ids.UUID{"thread": thread, "document": document, "participant": participant, "comment": comment}
	for name, read := range reads {
		gone, there := read(missing), read(existing[name])
		if !errors.Is(gone, apperrors.ErrPermissionDenied) || !errors.Is(there, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: a missing id answers %v and an existing one %v, want the same refusal", name, gone, there)
		}
	}
}

// A context read anchored on a room is refused cleanly: a room is not an anchor.
func TestAContextReadAnchoredOnARoomIsRefusedWithoutAServerError(t *testing.T) {
	e := SetupSearch(t)
	f := seedRooms(t, e)
	h := search.NewHandlers(e.DB(), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/records/deal_room/"+f.first.String()+"/context", nil)
	req = req.WithContext(e.Admin())
	rec := httptest.NewRecorder()
	h.GetRecordContext(rec, req, "deal_room", openapi_types.UUID(f.first), crmcontracts.GetRecordContextParams{})
	if rec.Code >= http.StatusInternalServerError || rec.Code < http.StatusBadRequest {
		t.Errorf("a context read on a room answered %d, want a client refusal", rec.Code)
	}
}
