// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The naming read's HTTP surface. SeatNames itself is covered in
// rosterteams_integration_test.go, which holds that a non-member is refused;
// these hold what the surface adds — which seats it names, and that it names
// them to a caller who administers nobody.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatNamePassword is shared across this file's invited seats: nothing here
// asserts about the password itself, so one value is enough to redeem with.
const seatNamePassword = "a colleague password!"

// inviteAndLogin provisions a member through the real invite → redeem →
// login path, the sequence rosterteams_integration_test.go spells out: a
// hand-built Identity would prove only that this file spelled a role key
// right, not that the seat can actually sign in.
func inviteAndLogin(t *testing.T, e *revocationEnv, email, name, role string) Identity {
	t.Helper()
	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: email, DisplayName: name, Role: role,
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	redeemCtx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws.UUID), ids.NewV7())
	if err := e.svc.RedeemPasswordReset(redeemCtx, rawToken, seatNamePassword); err != nil {
		t.Fatalf("redeeming %s's invite token: %v", email, err)
	}
	identity, _, err := e.svc.Login(principal.WithWorkspaceID(context.Background(), e.ws.UUID), email, seatNamePassword, noDevice)
	if err != nil {
		t.Fatalf("%s login: %v", email, err)
	}
	return identity
}

// archiveSeat marks a seat archived through the same raw statement
// oauth_useraccess_integration_test.go uses: the identity module writes no
// application-level archive path for app_user, only the invite/deactivate/
// reactivate lifecycle that goes through the service.
func archiveSeat(t *testing.T, e *revocationEnv, id ids.UserID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("archiving the seat: %v", err)
	}
}

// A seat that no longer works here still owns the records it made, so a rep
// looking at one gets a name rather than a uuid. This is the reading the roster
// walk could not produce: it stops at the seats that may still become active.
func TestANonAdminNamesADeactivatedColleague(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-deactivated")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One", "rep")

	departed, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "departed@acme.test", DisplayName: "Dana Kessler", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the colleague who later leaves: %v", err)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: departed}); err != nil {
		t.Fatalf("deactivating them: %v", err)
	}

	named, err := e.svc.SeatNames(e.wsCtx(rep), []ids.UserID{departed})
	if err != nil {
		t.Fatalf("naming a deactivated colleague: %v", err)
	}
	if named[departed.UUID] != "Dana Kessler" {
		t.Fatalf("a deactivated colleague is named; the read says %q", named[departed.UUID])
	}
}

// An archived seat is the one that stops resolving, which is what lets a name
// be withdrawn without rewriting the history that used it.
func TestAnArchivedSeatIsNotNamed(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-archived")

	gone, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "gone@acme.test", DisplayName: "Archived Seat", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the seat that is later archived: %v", err)
	}
	archiveSeat(t, e, gone)

	named, err := e.svc.SeatNames(e.wsCtx(e.admin), []ids.UserID{gone})
	if err != nil {
		t.Fatalf("asking about an archived seat: %v", err)
	}
	if _, present := named[gone.UUID]; present {
		t.Fatal("an archived seat was named")
	}
}
