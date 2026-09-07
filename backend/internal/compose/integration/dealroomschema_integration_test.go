// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The two schema guarantees the Deal Room writer LEANS ON, against a real
// Postgres and through the real writer.
//
// Both are load-bearing and neither was asserted anywhere. archiveRoomTx sets
// state and archived_at in one patch because a CHECK requires them to agree,
// and createRoomTx maps any unique violation to "this deal already has an
// active Deal Room" because one index is the only one an insert can trip.
// Written down in comments at both sites, and true only for as long as the
// schema says so — which is the half a comment cannot hold. A later edit
// splitting the patch, or a second unique index, would surface as a runtime
// constraint violation on the first archive anyone tried, or as a refusal
// naming the wrong cause.

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// openRoomOnANewDeal opens one live room and answers with the deal and the room,
// because both halves below need the deal id as well as the room's.
func openRoomOnANewDeal(t *testing.T, e *apptest.AppEnv, title string) (dealID, roomID string, version int64) {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID = apptest.CreateOpenDeal(t, e, stages)

	var room AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": title, "source": "ui",
	}, nil, &room); status != http.StatusCreated {
		t.Fatalf("open a room = %d %v", status, room)
	}
	roomID, _ = room["id"].(string)
	if v, ok := room["version"].(float64); ok {
		version = int64(v)
	}
	if roomID == "" {
		t.Fatalf("the room came back without an id: %v", room)
	}
	return dealID, roomID, version
}

// The archive moves the state and the stamp TOGETHER, and the database is what
// makes that mandatory rather than merely intended.
func TestArchivingARoomMovesItsStateAndItsStampTogether(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Room Schema", "ada@rooms.test", "Ada Admin")
	dealID, roomID, version := openRoomOnANewDeal(t, e, "Archive agreement")

	if status := e.Call(t, "DELETE", "/v1/deal-rooms/"+roomID, nil,
		map[string]string{"If-Match": fmt.Sprint(version)}, nil); status != http.StatusOK {
		t.Fatalf("archive = %d, want 200", status)
	}

	var state string
	var stamped bool
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT state, archived_at IS NOT NULL FROM deal_room WHERE id = $1`, roomID,
	).Scan(&state, &stamped); err != nil {
		t.Fatalf("reading the archived room: %v", err)
	}
	if state != "archived" || !stamped {
		t.Errorf("after archiving, state=%q archived_at set=%v — want archived and set. A row that "+
			"reads live to the state machine and archived to every list query is the split this "+
			"patch is written as one to prevent", state, stamped)
	}

	// And the reason the writer CAN be simple: half an archive is refused by the
	// database. Without this the single patch is a convention, and a later edit
	// splitting it would fail on the first archive anyone tried rather than here.
	// The deal is free again now that the first room is archived, so the probe
	// row costs no second organization.
	var reopened AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": "Half an archive", "source": "ui",
	}, nil, &reopened); status != http.StatusCreated {
		t.Fatalf("reopen after archiving = %d %v", status, reopened)
	}
	roomID2, _ := reopened["id"].(string)
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE deal_room SET state = 'archived' WHERE id = $1`, roomID2); err == nil {
		t.Error("the database accepted state='archived' with no archived_at — deal_room_archived_agrees " +
			"is what lets archiveRoomTx set the two in one patch and trust the result, and it is gone")
	}
}

// A second room on a deal that still has an active one is refused as exactly
// that, and archiving the first frees the deal — which is the sentence the
// refusal tells the caller to act on.
func TestASecondRoomOnALiveDealIsRefusedAsAlreadyOpen(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Room Conflict", "ada@rooms.test", "Ada Admin")
	dealID, roomID, version := openRoomOnANewDeal(t, e, "The first room")

	var problem AnyMap
	status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": "The second room", "source": "ui",
	}, nil, &problem)
	// 409 is what crm.yaml declares for this, and what errRoomAlreadyOpen
	// unwraps to. It answered 422 until #2269: httperr read the message fault
	// before the sentinel, so the Unwrap was unreachable and a caller branching
	// on status was told the wrong thing about whether retrying could help.
	if status != http.StatusConflict {
		t.Fatalf("a second room on a live deal = %d %v, want 409", status, problem)
	}
	// The CODE, not the status: a caller branches on it, and a unique violation
	// from some other index would arrive as a 409 saying the same wrong thing.
	if problem["code"] != "deal_room_already_open" {
		t.Errorf("the refusal carried code %v, want \"deal_room_already_open\"", problem["code"])
	}

	if status := e.Call(t, "DELETE", "/v1/deal-rooms/"+roomID, nil,
		map[string]string{"If-Match": fmt.Sprint(version)}, nil); status != http.StatusOK {
		t.Fatalf("archive the first room = %d, want 200", status)
	}
	var second AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": "The second room", "source": "ui",
	}, nil, &second); status != http.StatusCreated {
		t.Errorf("after archiving the first, a second room = %d, want 201 — the refusal tells the "+
			"caller archiving frees the deal, and it has to be true", status)
	}
}

// The mapping from "unique violation" to "already open" is only honest while
// ONE unique index can refuse an insert into deal_room.
//
// createRoomTx cannot tell which constraint bit it, so a second one added later
// would make every refusal it causes say "this deal already has an active Deal
// Room" — a sentence naming a cause that is not the cause, telling the caller
// to archive a room that would not help. Asked of the catalog rather than the
// migration text, because what binds is what the database has.
func TestTheActiveRoomIndexIsTheOnlyUniqueRefusalACreateCanMean(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Room Indexes", "ada@rooms.test", "Ada Admin")

	rows, err := e.Owner.Query(t.Context(),
		`SELECT i.relname
		   FROM pg_index x
		   JOIN pg_class i ON i.oid = x.indexrelid
		   JOIN pg_class c ON c.oid = x.indrelid
		  WHERE c.relname = 'deal_room' AND x.indisunique`)
	if err != nil {
		t.Fatalf("reading deal_room's unique indexes: %v", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning an index name: %v", err)
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading deal_room's unique indexes: %v", err)
	}
	sort.Strings(found)

	// The primary key cannot be the one: the writer generates the id, so an
	// insert tripping it is a collision no caller can act on.
	want := []string{"deal_room_pkey", "uq_deal_room_active"}
	if len(found) != len(want) {
		t.Fatalf("deal_room carries %d unique index(es), %v, and this test is written for %v.\n"+
			"A new one makes createRoomTx's storekit.IsUniqueViolation → errRoomAlreadyOpen mapping "+
			"name the wrong cause: the refusal would tell a caller to archive a room that is not the "+
			"problem. Either map the constraint by name, or add the index here having decided that "+
			"\"already open\" is still the true thing to say.", len(found), found, want)
	}
	for i, name := range want {
		if found[i] != name {
			t.Errorf("deal_room's unique indexes are %v, want %v", found, want)
			break
		}
	}
}

// A refusal about the room's STATE answers 409 too, which is the other half of
// the same defect: every stateError unwraps to ErrConflict and every one of
// them answered 422.
//
// Inviting into a closed room is the case #2269 reproduced against a running
// stack. It is a different constructor from the create conflict above, and
// worth its own arm — the fix is in shared code, so one passing and the other
// not would say the mapping is per-error rather than per-sentinel.
func TestARefusalAboutTheRoomsStateAnswersConflictToo(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Room State", "ada@rooms.test", "Ada Admin")
	_, roomID, version := openRoomOnANewDeal(t, e, "Closed to invitations")

	if status := e.Call(t, "POST", "/v1/deal-rooms/"+roomID+"/close", AnyMap{},
		map[string]string{"If-Match": fmt.Sprint(version)}, nil); status != http.StatusOK {
		t.Fatalf("close = %d, want 200", status)
	}

	var problem AnyMap
	status := e.Call(t, "POST", "/v1/deal-rooms/"+roomID+"/participants", AnyMap{
		"full_name": "Late Buyer", "email": "late@buyer.example", "capability": "comment", "source": "ui",
	}, nil, &problem)
	if status != http.StatusConflict {
		t.Errorf("inviting into a closed room = %d %v, want 409 — the contract declares it and the "+
			"refusal unwraps to ErrConflict", status, problem)
	}
	if problem["code"] != "deal_room_not_admitting" {
		t.Errorf("the refusal carried code %v, want \"deal_room_not_admitting\" — the status is what "+
			"#2269 moved, and the module keeps its own code and detail", problem["code"])
	}
}
