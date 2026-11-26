// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/search"
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

func roomHitIDs(t *testing.T, e *SearchEnv, ctx context.Context, query string) []ids.UUID {
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

func searchEnvRoomList(t *testing.T, e *SearchEnv, ctx context.Context) []ids.UUID {
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
		got := roomHitIDs(t, e, ctx, "harbour")
		sortIDs(got)
		want := []ids.UUID{f.first, f.second}
		sortIDs(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: search found %v, want the two live rooms %v", name, got, want)
		}
		listed := searchEnvRoomList(t, e, ctx)
		sortIDs(listed)
		if !slices.Equal(got, listed) {
			t.Errorf("%s: search found %v but the room list shows %v", name, got, listed)
		}
		if byMessage := roomHitIDs(t, e, ctx, "walkthrough"); !slices.Equal(byMessage, []ids.UUID{f.first}) {
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
	if got := roomHitIDs(t, e, ctx, "harbour"); len(got) != 0 {
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
