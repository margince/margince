// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWorkAuthorityIncludesInvitedHumansWithoutAdmittingThemAsLiveMembers(t *testing.T) {
	e := setupRevocationEnv(t, "work-authority")
	invited := e.seat(t, "invited", "rep")
	gone := e.seat(t, "gone", "rep")
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: gone.UserID}); err != nil {
		t.Fatal(err)
	}
	archived := e.seat(t, "archived", "rep")
	archiveSeat(t, e, archived.UserID)
	agent, err := e.svc.DeclareServiceMember(e.wsCtx(e.admin), e.admin, ServiceMemberInput{Extension: "synthetic", DisplayName: "Synthetic feed"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.wsCtx(e.admin)
	for _, tc := range []struct {
		name       string
		user       ids.UserID
		work, live bool
	}{
		{"active", e.admin.UserID, true, true},
		{"invited", invited.UserID, true, false},
		{"deactivated", gone.UserID, false, false},
		{"archived", archived.UserID, false, false},
		{"agent", agent, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
				member, eligible, err := WorkPrincipalTx(ctx, tx, tc.user.UUID)
				if err != nil {
					return err
				}
				if eligible != tc.work {
					t.Fatalf("work eligible=%v, want %v", eligible, tc.work)
				}
				_, live, err := MemberPrincipalTx(ctx, tx, tc.user.UUID)
				if err != nil {
					return err
				}
				if tc.name != "agent" && live != tc.live {
					t.Fatalf("live member=%v, want %v", live, tc.live)
				}
				if eligible {
					grants, err := loadGrantsFor(ctx, tx, []ids.UUID{tc.user.UUID})
					if err != nil {
						return err
					}
					if member.Type != principal.PrincipalHuman || member.UserID != tc.user.UUID || !reflect.DeepEqual(member.Permissions, grants[tc.user.UUID].perms) {
						t.Fatalf("work authority does not retain the member's actual grants: %+v", member)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
