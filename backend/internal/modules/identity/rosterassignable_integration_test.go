// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The roster marks each member as an owner change would answer for this caller.
// The admin may hand work to an active rep. A rep may not hand work to the admin
// outside their scope. Nobody may hand work to a member who left.
func TestTheRosterSaysWhoTheCallerMayMakeAnOwner(t *testing.T) {
	e := setupRevocationEnv(t, "roster-assignable")
	ctx := context.Background()

	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "rep@acme.test", DisplayName: "Rep One", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the rep: %v", err)
	}
	if err := e.svc.RedeemPasswordReset(principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7()), rawToken, "a rep password!"); err != nil {
		t.Fatalf("redeeming the invite token: %v", err)
	}
	rep, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID), "rep@acme.test", "a rep password!", noDevice)
	if err != nil {
		t.Fatalf("rep login: %v", err)
	}

	assertAssignable(t, "the admin, about the active rep", findRosterUser(t, e, e.admin, true, rep.UserID.UUID), true)
	assertAssignable(t, "the rep, about themselves", findRosterUser(t, e, rep, false, rep.UserID.UUID), true)
	outside := findRosterUser(t, e, rep, false, e.admin.UserID.UUID)
	assertAssignable(t, "the rep, about the admin outside their scope", outside, false)
	assertReceivesHandOver(t, "the rep, about the admin outside their scope", outside, true)

	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: rep.UserID}); err != nil {
		t.Fatalf("deactivating the rep: %v", err)
	}
	left := findRosterUser(t, e, e.admin, true, rep.UserID.UUID)
	assertAssignable(t, "the admin, about the deactivated rep", left, false)
	assertReceivesHandOver(t, "the admin, about the deactivated rep", left, false)
}

func assertAssignable(t *testing.T, who string, row userRow, want bool) {
	t.Helper()
	if row.Assignable == nil {
		t.Fatalf("%s: the roster did not say whether the member may own records", who)
	}
	if *row.Assignable != want {
		t.Errorf("%s: assignable = %v, want %v", who, *row.Assignable, want)
	}
}

// assertReceivesHandOver reads the offer picker's answer, which consent frees
// from the caller's own scope.
func assertReceivesHandOver(t *testing.T, who string, row userRow, want bool) {
	t.Helper()
	if row.ReceivesHandOver == nil {
		t.Fatalf("%s: the roster did not say whether the member may be offered a record", who)
	}
	if *row.ReceivesHandOver != want {
		t.Errorf("%s: receives_handover = %v, want %v", who, *row.ReceivesHandOver, want)
	}
}
