// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// ownedKinds are the records that carry an owner, which row scope is about, by
// the collection that serves each.
//
// gatekit:fixture the route that reads each owned record: expected data, not a waived cost
var ownedKinds = map[string]string{
	"company": "/v1/companies/", "contact": "/v1/contacts/", "deal": "/v1/deals/", "lead": "/v1/leads/", "project": "/v1/projects/",
}

// A call on a colleague's record ran and left it as it was, or changed it.
// Any other answer is the reason the call stopped.
const (
	colleagueRan     = "ran"
	colleagueChanged = "changed"
)

// crossOwnerPass calls every tool that takes an owned record, on records a
// colleague owns, as each role whose row scope stops short of all. Every
// granted seat reads an identity record and may act on its own work beside it.
// So a call may run, and it must not change the colleague's record. It answers
// each call's row with what each such role was told.
func crossOwnerPass(t *testing.T, admin *apptest.AppEnv, seats []roleSeat, calls []toolRoleCall) map[string]map[string]string {
	t.Helper()
	owned := callsTakingOwnedRecords(t, calls)
	if len(owned) == 0 {
		t.Fatal("no call takes a record with an owner, so the cross-owner pass measured nothing")
	}
	colleague := seededID(t, admin, "/v1/users", integration.AnyMap{"email": "colleague@role-sweep.test", "display_name": "Colleague", "role": "rep"})
	scoped := slices.DeleteFunc(slices.Clone(seats), func(seat roleSeat) bool { return seat.role.RowScope == "all" })
	if len(scoped) == 0 {
		t.Fatal("every seeded role reads all rows, so the cross-owner pass measured nothing")
	}
	byRole := make([]map[string]string, len(scoped))
	eachSeat(t, "colleague", scoped, func(t *testing.T, i int, seat roleSeat) {
		byRole[i] = colleagueAnswers(t, admin, seat, owned, colleague)
	})
	answers := map[string]map[string]string{}
	for i, seat := range scoped {
		for row, answer := range byRole[i] {
			if answers[row] == nil {
				answers[row] = map[string]string{}
			}
			answers[row][seat.role.Key] = answer
		}
	}
	return answers
}

// colleagueAnswers calls each owned-record call as the seat's role on records
// the colleague owns, and answers what each row was told. The admin reads the
// records before and after, as the scoped seat may not see them at all.
func colleagueAnswers(t *testing.T, admin *apptest.AppEnv, seat roleSeat, owned []toolRoleCall, colleague string) map[string]string {
	t.Helper()
	apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	clients := passportClients(t, seat.e, seat.role.Key+" colleague", 1+len(owned)/toolRoleCallsPerPassport)
	pool := seedToolRoleRecords(t, seat.e, owned, colleague)
	apptest.AssignRoleTo(t, seat.e, seat.email, seat.role.Key)
	defer apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	answers := map[string]string{}
	for n, call := range owned {
		pool.taken = map[string][]string{}
		args := call.args(pool)
		before := ownedRecordStates(t, admin, pool.taken)
		got := clients[n/toolRoleCallsPerPassport].Call(t, call.verb, args)
		if got.IsError && isBudgetRefusal(got.Text) {
			t.Fatalf("%s as %s spent the call budget, so the rest would read as measured: %s", call.verb, seat.role.Key, got.Text)
		}
		answer := colleagueAnswer(got)
		if changed := changedRecord(before, ownedRecordStates(t, admin, pool.taken)); changed != "" {
			answer = colleagueChanged
			t.Errorf("%s as %s (row scope %s) changed %s, which a colleague owns: %.300s", call.row, seat.role.Key, seat.role.RowScope, changed, got.Text)
		}
		answers[call.row] = answer
	}
	return answers
}

func callsTakingOwnedRecords(t *testing.T, calls []toolRoleCall) []toolRoleCall {
	t.Helper()
	var owned []toolRoleCall
	for _, call := range calls {
		counting := countingPool(t)
		call.args(counting)
		for kind := range ownedKinds {
			if counting.wanted[kind] > 0 {
				owned = append(owned, call)
				break
			}
		}
	}
	return owned
}

// ownedRecordStates reads each owned record a call took through its own route:
// the status and the fields every write moves.
func ownedRecordStates(t *testing.T, e *apptest.AppEnv, taken map[string][]string) map[string]string {
	t.Helper()
	states := map[string]string{}
	for kind, path := range ownedKinds {
		for _, id := range taken[kind] {
			var record struct {
				Version    json.RawMessage `json:"version"`
				UpdatedAt  string          `json:"updated_at"`
				ArchivedAt string          `json:"archived_at"`
				OwnerID    string          `json:"owner_id"`
			}
			status := e.Call(t, "GET", path+id, nil, nil, &record)
			states[kind+" "+id] = fmt.Sprint(status, string(record.Version), record.UpdatedAt, record.ArchivedAt, record.OwnerID)
		}
	}
	return states
}

func changedRecord(before, after map[string]string) string {
	for record, state := range before {
		if after[record] != state {
			return record
		}
	}
	return ""
}

func colleagueAnswer(got apptest.MCPResult) string {
	if !got.IsError {
		return colleagueRan
	}
	return stopReason(got.Text)
}
