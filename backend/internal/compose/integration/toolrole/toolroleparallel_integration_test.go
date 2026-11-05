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

// seatPassword is the password every seat the sweep invites sets.
const seatPassword = "role-sweep-seat-password"

// signedInAs is a second session of the harness, as a new active admin who
// accepted the invite. It has the calendar and the send-capable mailbox the
// bootstrap admin has.
func signedInAs(t *testing.T, admin *apptest.AppEnv, email string) *apptest.AppEnv {
	t.Helper()
	id := seededID(t, admin, "/v1/users", integration.AnyMap{"email": email, "display_name": "Role " + email, "role": "admin"})
	apptest.SetPasswordByLink(t, admin, id, seatPassword)
	seat := admin.AsSecondSeat(t, email, seatPassword)
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
	call toolRoleCall
	got  apptest.MCPResult
	run  auth.RecordedCall
	ran  bool
	// releaseRefused is what refused the human the release of a staged call.
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

// aloneVerbs change what the installation has one of: the input check that
// runs, the anchor company and its logos, a settings page, the review template.
// No other role's such call or seed runs meanwhile, or one role's write would
// be another's answer.
var aloneVerbs = map[string]bool{
	"start_forecast_assurance_run": true, "confirm_anchor_company_site_read": true,
	"put_anchor_company": true, "update_activity_review_template": true,
	"delete_anchor_company_logo": true, "delete_anchor_company_logo_icon": true, "delete_legal_entity_logo": true,
	"update_contact_case_settings": true, "update_follow_up_settings": true, "update_lead_settings": true,
	"update_legal_review_settings": true,
}

// runsAlone is whether call changes or reads what the installation has one of:
// a verb in aloneVerbs, or a call taking a kind of installationKinds.
func runsAlone(t *testing.T, call toolRoleCall) bool {
	t.Helper()
	if aloneVerbs[call.verb] {
		return true
	}
	counting := countingPool(t)
	call.args(counting)
	for kind := range counting.wanted {
		if installationKinds[kind] {
			return true
		}
	}
	return false
}

// callAsSeat makes one call as the seat's role. A call that runs alone has its
// records seeded as the admin, under the installation's lock, and runs there.
func callAsSeat(t *testing.T, seat roleSeat, client *apptest.MCPClient, call toolRoleCall, pool *recordPool) (apptest.MCPResult, []string) {
	t.Helper()
	if !runsAlone(t, call) {
		return callThroughApproval(t, seat.e, client, call.verb, call.args(pool))
	}
	installationSeeds.Lock()
	defer installationSeeds.Unlock()
	apptest.AssignRoleTo(t, seat.e, seat.email, "admin")
	args := call.args(pool)
	apptest.AssignRoleTo(t, seat.e, seat.email, seat.role.Key)
	return callThroughApproval(t, seat.e, client, call.verb, args)
}
