// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// ownedKinds are the records that carry an owner, which row scope is about, by
// the collection that serves each.
var ownedKinds = map[string]string{
	"company": "/v1/companies/", "contact": "/v1/contacts/", "deal": "/v1/deals/", "lead": "/v1/leads/", "project": "/v1/projects/",
}

// What a scoped role is answered on a colleague's record, beside the reason a
// call stopped: it ran and left the record as it was, or it changed it.
const (
	colleagueRan     = "ran"
	colleagueChanged = "changed"
)

// crossOwnerPass calls every tool that takes an owned record, on records a
// colleague owns, as each role whose row scope stops short of all. Every
// granted seat reads an identity record and may act on its own work beside one,
// so a call may run; it must not change the colleague's record. It answers each
// call's row with what each such role was told.
func crossOwnerPass(t *testing.T, e *apptest.AppEnv, roles []seededRole, calls []toolRoleCall) map[string]map[string]string {
	t.Helper()
	owned := callsTakingOwnedRecords(calls)
	if len(owned) == 0 {
		t.Fatal("no call takes a record with an owner, so the cross-owner pass measured nothing")
	}
	assignRoleTo(t, e, toolRoleUser, "admin")
	colleague := seededID(t, e, "/v1/users", AnyMap{"email": "colleague@role-sweep.test", "display_name": "Colleague", "role": "rep"})
	answers := map[string]map[string]string{}
	scoped := 0
	for _, role := range roles {
		if role.rowScope == "all" {
			continue
		}
		scoped++
		assignRoleTo(t, e, toolRoleUser, "admin")
		clients := passportClients(t, e, role.key+" colleague", 1+len(owned)/toolRoleCallsPerPassport)
		pool := seedToolRoleRecords(t, e, owned, colleague)
		assignRoleTo(t, e, toolRoleUser, role.key)
		for n, call := range owned {
			pool.taken = map[string][]string{}
			args := call.args(pool)
			before := ownedRecordStates(t, e, pool.taken)
			got := clients[n/toolRoleCallsPerPassport].Call(t, call.verb, args)
			if got.IsError && isBudgetRefusal(got.Text) {
				t.Fatalf("%s as %s spent the call budget, so the rest would read as measured: %s", call.verb, role.key, got.Text)
			}
			answer := colleagueAnswer(got)
			if changed := changedRecord(before, ownedRecordStates(t, e, pool.taken)); changed != "" {
				answer = colleagueChanged
				t.Errorf("%s as %s (row scope %s) changed %s, which a colleague owns: %.300s", call.row, role.key, role.rowScope, changed, got.Text)
			}
			if answers[call.row] == nil {
				answers[call.row] = map[string]string{}
			}
			answers[call.row][role.key] = answer
		}
	}
	if scoped == 0 {
		t.Fatal("every seeded role reads all rows, so the cross-owner pass measured nothing")
	}
	return answers
}

func callsTakingOwnedRecords(calls []toolRoleCall) []toolRoleCall {
	var owned []toolRoleCall
	for _, call := range calls {
		counting := countingPool()
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
