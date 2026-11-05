// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// A seeded role's seat: its own signed-in user, so each role's passes run at
// once and no role waits on another's role change. The admin's seat is the
// bootstrap admin's own.
type roleSeat struct {
	role  apptest.SeededRole
	e     *apptest.AppEnv
	email string
	human string
}

// roleSeats signs in one user per seeded role. Each holds admin while its
// records are seeded, and its role while its calls run. One user did both when
// the roles ran one after another.
func roleSeats(t *testing.T, admin *apptest.AppEnv, roles []apptest.SeededRole) []roleSeat {
	t.Helper()
	seats := make([]roleSeat, len(roles))
	for i, role := range roles {
		if role.Key == "admin" {
			seats[i] = roleSeat{role: role, e: admin, email: toolRoleUser, human: apptest.SessionUserID(t, admin)}
			continue
		}
		email := "tool-role-" + strings.ReplaceAll(role.Key, "_", "-") + "@fable.test"
		seats[i] = roleSeat{role: role, e: signedInAs(t, admin, email), email: email}
		seats[i].human = apptest.SessionUserID(t, seats[i].e)
	}
	return seats
}

// signedInAs is a second session of the harness, as a new active admin. It has
// the calendar and the send-capable mailbox the bootstrap admin has. No route
// makes a seat active but the invitee's own password link. So the seat takes
// the bootstrap admin's password, the way an accepted invite sets one.
func signedInAs(t *testing.T, admin *apptest.AppEnv, email string) *apptest.AppEnv {
	t.Helper()
	id := seededID(t, admin, "/v1/users", integration.AnyMap{"email": email, "display_name": "Role " + email, "role": "admin"})
	if _, err := admin.Owner.Exec(t.Context(), `UPDATE app_user SET status = 'active',
		password_hash = (SELECT password_hash FROM app_user WHERE id = $1) WHERE id = $2`, apptest.SessionUserID(t, admin), id); err != nil {
		t.Fatalf("activating %s: %v", email, err)
	}
	seat := admin.AsSecondSeat(t, email, "correct-horse-battery")
	apptest.EnableGoogleBookingPage(t, seat)
	sendCapableMailbox(t, seat)
	return seat
}

// eachSeat runs pass once per seat at the same time, and waits for all.
func eachSeat(t *testing.T, name string, seats []roleSeat, pass func(t *testing.T, i int, seat roleSeat)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		for i, seat := range seats {
			t.Run(seat.role.Key, func(t *testing.T) {
				t.Parallel()
				pass(t, i, seat)
			})
		}
	})
}

// sweptCall is one call as one role made it, kept for the merge.
type sweptCall struct {
	call           toolRoleCall
	got            apptest.MCPResult
	run            auth.RecordedCall
	ran            bool
	releaseRefused []string
}

// sweepSeat seeds the role's records and makes every call as the role.
func sweepSeat(t *testing.T, seat roleSeat, recorded *toolCheckRecorder, calls []toolRoleCall, elsewhere map[string]elsewhereRow) []sweptCall {
	t.Helper()
	apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	clients := passportClients(t, seat.e, seat.role.Key, 1+len(calls)/toolRoleCallsPerPassport)
	pool := seedToolRoleRecords(t, seat.e, calls, "")
	apptest.AssignRoleTo(t, seat.e, seat.email, seat.role.Key)
	defer apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	var swept []sweptCall
	for n, call := range calls {
		if _, measured := elsewhere[call.verb]; measured {
			continue
		}
		client := clients[n/toolRoleCallsPerPassport]
		pool.caller = client.Bearer()
		got, releaseRefused := callAsSeat(t, seat, client, call, pool)
		if got.IsError && strings.Contains(got.Text, "human-only operation") {
			t.Errorf("%s is offered to agents and its route refuses every agent: %s", call.verb, got.Text)
		}
		if got.IsError && isBudgetRefusal(got.Text) {
			t.Fatalf("%s as %s spent the call budget, so the rest would read as measured: %s", call.verb, seat.role.Key, got.Text)
		}
		run, ran := recorded.take(call.verb, seat.human)
		swept = append(swept, sweptCall{call: call, got: got, run: run, ran: ran, releaseRefused: releaseRefused})
	}
	return swept
}

// aloneVerbs act on what the installation has one of: the input check that
// runs, the anchor company a site read proposes for, the review template. Each
// role's call is made with its state seeded just before. No other role's such
// call or seed runs meanwhile, or one role's write would be another's answer.
//
// gatekit:fixture the verbs whose state the whole installation shares: expected data, not a waived cost
var aloneVerbs = map[string]bool{
	"start_forecast_assurance_run": true, "confirm_anchor_company_site_read": true,
	"put_anchor_company": true, "update_activity_review_template": true,
}

// callAsSeat makes one call as the seat's role. A call in aloneVerbs has its
// records seeded as the admin, under the installation's lock, and runs there.
func callAsSeat(t *testing.T, seat roleSeat, client *apptest.MCPClient, call toolRoleCall, pool *recordPool) (apptest.MCPResult, []string) {
	t.Helper()
	if !aloneVerbs[call.verb] {
		return callThroughApproval(t, seat.e, client, call.verb, call.args(pool))
	}
	installationSeeds.Lock()
	defer installationSeeds.Unlock()
	apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	args := call.args(pool)
	apptest.AssignRoleTo(t, seat.e, seat.email, seat.role.Key)
	return callThroughApproval(t, seat.e, client, call.verb, args)
}
