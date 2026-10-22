// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// An admin removes the second factor of a member who lost it. The member's
// password signs them in again and their old sessions are gone. The ledger and
// the bus name the admin. The roster offers the action only where the write
// accepts it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// enrolMember gives the env's member a confirmed factor and returns the token
// of a session they opened before it.
func (e *revocationEnv) enrolMember(t *testing.T) string {
	t.Helper()
	e.withVault()
	before := e.signIn(t, "member's old laptop")
	ctx := e.asMember()
	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code, err := totpCodeAt(secret, e.svc.now(), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ConfirmTOTP(ctx, code, hashToken(before)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	return before
}

func TestAnAdminRemovesALockedOutMembersFactorAndTheirPasswordSignsThemIn(t *testing.T) {
	e := setupRevocationEnv(t, "member-mfa-remove")
	session := e.enrolMember(t)
	if _, _, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice); !errors.Is(err, errMFARequired) {
		t.Fatalf("login before removal: err=%v, want the factor challenge", err)
	}

	if err := e.svc.RemoveMemberMFA(e.wsCtx(e.admin), e.admin, e.member.UserID); err != nil {
		t.Fatalf("RemoveMemberMFA: %v", err)
	}

	if status := statusOf(t, e); status.Enrolled || status.RecoveryCodesLeft != 0 {
		t.Errorf("after removal the member still holds %+v", status)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), session); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the member's session survived the removal: err=%v, want ErrNotFound", err)
	}
	if _, _, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice); err != nil {
		t.Errorf("the password alone does not sign the member in after removal: %v", err)
	}

	var capturedBy string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT actor_id FROM audit_log WHERE action = 'mfa_removed' AND entity_id = $1`,
		e.member.UserID).Scan(&capturedBy); err != nil {
		t.Fatalf("mfa_removed audit row: %v", err)
	}
	if capturedBy != "human:"+e.admin.UserID.String() {
		t.Errorf("the removal is audited as %q, want the admin", capturedBy)
	}
	envs := e.identityEvents(t, "user.mfa_removed", e.member.UserID.UUID)
	if len(envs) != 1 {
		t.Fatalf("user.mfa_removed staged %d times, want once", len(envs))
	}
}

// A pending enrolment guards no sign-in, so it is neither removed nor reason
// to sign the member out.
func TestRemovingAMembersFactorIsANoOpWhereNoneIsConfirmed(t *testing.T) {
	e := setupRevocationEnv(t, "member-mfa-none")
	e.withVault()
	session := e.signIn(t, "member's laptop")
	if err := e.svc.RemoveMemberMFA(e.wsCtx(e.admin), e.admin, e.member.UserID); err != nil {
		t.Fatalf("RemoveMemberMFA on a member with no factor: %v", err)
	}
	if _, err := e.svc.StartTOTPEnrolment(e.asMember()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := e.svc.RemoveMemberMFA(e.wsCtx(e.admin), e.admin, e.member.UserID); err != nil {
		t.Fatalf("RemoveMemberMFA on a pending enrolment: %v", err)
	}
	if status := statusOf(t, e); !status.Enrolled || status.Confirmed {
		t.Errorf("the pending enrolment did not survive: %+v", status)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), session); err != nil {
		t.Errorf("a no-op removal signed the member out: %v", err)
	}
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'mfa_removed'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a removal that changed nothing wrote %d audit rows", rows)
	}
}

func TestRemovingAFactorIsRefusedToANonAdminAndForOnesOwn(t *testing.T) {
	e := setupRevocationEnv(t, "member-mfa-refused")
	e.enrolMember(t)
	rep := e.seat(t, "plain-rep", "rep")
	readSeat := e.admin
	readSeat.SeatType = string(principal.SeatRead)
	for name, tc := range map[string]struct {
		actor  Identity
		target ids.UserID
		want   error
	}{
		"a member without user_admin": {rep, e.member.UserID, apperrors.ErrPermissionDenied},
		"the admin's own factor":      {e.admin, e.admin.UserID, errOwnFactor},
		"an unknown member":           {e.admin, ids.From[ids.UserKind](ids.NewV7()), apperrors.ErrNotFound},
		"an admin on a read seat":     {readSeat, e.member.UserID, apperrors.ErrSeatTierInsufficient},
	} {
		if err := e.svc.RemoveMemberMFA(e.wsCtx(tc.actor), tc.actor, tc.target); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v, want %v", name, err, tc.want)
		}
	}
	if !statusOf(t, e).Confirmed {
		t.Error("a refused removal took the member's factor anyway")
	}
}

// The roster offers remove_mfa on the member holding a factor and nowhere else,
// and the handler accepts the offered call.
func TestTheRosterOffersFactorRemovalOnlyForAMemberWhoHasOne(t *testing.T) {
	e := setupRevocationEnv(t, "member-mfa-offer")
	e.enrolMember(t)
	h := NewHandlers(e.svc)
	offers := rosterActions(t, e, h, e.admin)
	remove := crmcontracts.UserAllowedActions(actionRemoveMFA)
	if slices.Contains(offers[e.admin.UserID.UUID], remove) {
		t.Error("remove_mfa is offered on the admin's own row")
	}
	if !slices.Contains(offers[e.member.UserID.UUID], remove) {
		t.Fatalf("remove_mfa is not offered on the enrolled member: %v", offers[e.member.UserID.UUID])
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/users/"+e.member.UserID.String()+"/mfa", nil).
		WithContext(withIdentity(e.wsCtx(e.admin), e.admin))
	h.RemoveUserMfa(rec, req, crmcontracts.Id(e.member.UserID.UUID))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /users/{id}/mfa = %d: %s", rec.Code, rec.Body)
	}
	if slices.Contains(rosterActions(t, e, h, e.admin)[e.member.UserID.UUID], remove) {
		t.Error("remove_mfa is still offered once the factor is gone")
	}
}

func TestTheFactorRemovalHandlerAnswersARefusalAsTheContractSays(t *testing.T) {
	e := setupRevocationEnv(t, "member-mfa-http")
	e.enrolMember(t)
	h := NewHandlers(e.svc)
	rep := e.seat(t, "http-rep", "rep")
	for name, tc := range map[string]struct {
		ctx  context.Context
		want int
	}{
		"no signed-in caller":         {e.wsOnlyCtx(), http.StatusUnauthorized},
		"a member without user_admin": {withIdentity(e.wsCtx(rep), rep), http.StatusForbidden},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/v1/users/"+e.member.UserID.String()+"/mfa", nil).
			WithContext(tc.ctx)
		h.RemoveUserMfa(rec, req, crmcontracts.Id(e.member.UserID.UUID))
		if rec.Code != tc.want {
			t.Errorf("%s: DELETE /users/{id}/mfa = %d, want %d", name, rec.Code, tc.want)
		}
	}
}
