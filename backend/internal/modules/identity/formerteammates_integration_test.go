// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A teammate who left is a former teammate. One who stays is not. A colleague
// who left another team is nobody's former teammate here.
func TestAFormerTeammateIsSomebodyWhoLeftTheReadersTeam(t *testing.T) {
	e := setupRevocationEnv(t, "former-teammates")
	sales, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "DACH Sales")
	if err != nil {
		t.Fatalf("creating the team: %v", err)
	}
	other, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "Nordics")
	if err != nil {
		t.Fatalf("creating the other team: %v", err)
	}
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, sales.ID, e.admin.UserID.UUID, true); err != nil {
		t.Fatalf("adding the admin to the team: %v", err)
	}
	stays := e.seat(t, "stays", "rep", sales.ID)
	left := e.seat(t, "left", "rep", sales.ID)
	elsewhere := e.seat(t, "elsewhere", "rep", other.ID)
	for _, gone := range []Identity{left, elsewhere} {
		if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: gone.UserID}); err != nil {
			t.Fatalf("deactivating %s: %v", gone.UserID, err)
		}
	}

	former, err := e.svc.FormerTeammatesOfCaller(e.wsCtx(e.admin))
	if err != nil {
		t.Fatalf("reading former teammates: %v", err)
	}
	if !slices.Equal(former, []ids.UUID{left.UserID.UUID}) {
		t.Errorf("former teammates = %v, want only %s (left the reader's team); %s stays, %s left another team",
			former, left.UserID, stays.UserID, elsewhere.UserID)
	}
}
