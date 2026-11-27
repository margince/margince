// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestAMemberEditKeepsAnUnchangedLegacyEmail(t *testing.T) {
	e := setupRevocationEnv(t, "legacy-member-email")
	address := strings.Repeat("a", values.MaxEmailLength) + "@legacy.test"
	seedHistoricalMemberEmail(t, e, e.member.UserID, address)
	if _, _, err := e.svc.Login(e.wsOnlyCtx(), address, memberPassword, noDevice); err != nil {
		t.Fatal(err)
	}
	ctx := e.wsCtx(e.admin)
	row, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	team, err := e.svc.CreateTeam(ctx, e.admin, "Delivery")
	if err != nil {
		t.Fatal(err)
	}
	err = e.svc.UpdateMember(ctx, e.admin, e.member.UserID, MemberEdit{
		Name: "Corrected Name", Email: strings.ToUpper(address), Teams: new([]ids.UUID{team.ID}), ExpectedUpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := e.svc.GetUser(ctx, e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DisplayName != "Corrected Name" || saved.Email != address || len(saved.TeamIDs) != 1 || saved.TeamIDs[0] != team.ID {
		t.Fatalf("saved member = %+v", saved)
	}
	if len(e.identityEvents(t, "user.updated", e.member.UserID.UUID)) != 1 {
		t.Fatal("member edit did not publish its event")
	}
}

func TestAChangedMemberEmailMustMeetTheAddressBound(t *testing.T) {
	e := setupRevocationEnv(t, "bounded-member-email")
	ctx := e.wsCtx(e.admin)
	member, err := e.svc.CreateFormerMember(ctx, e.admin, FormerMemberInput{Email: "imported@example.test", DisplayName: "Imported colleague"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	err = e.svc.UpdateMember(ctx, e.admin, member, MemberEdit{
		Name: "Must Roll Back", Email: strings.Repeat("b", values.MaxEmailLength) + "@legacy.test", ExpectedUpdatedAt: row.UpdatedAt,
	})
	fault, ok := errors.AsType[*values.ParseError](err)
	if !ok || fault.Field != "email" || fault.Code != "too_long" || fault.Max != values.MaxEmailLength {
		t.Fatalf("email edit = %v, want email/too_long", err)
	}
	saved, err := e.svc.GetUser(ctx, e.admin, member)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Email != row.Email || saved.DisplayName != row.DisplayName || !saved.UpdatedAt.Equal(row.UpdatedAt) {
		t.Fatal("refused email edit changed the member")
	}
}

func TestPasswordResetFindsALegacyEmailPastTheWriteBound(t *testing.T) {
	e := setupRevocationEnv(t, "legacy-reset-email")
	address := strings.Repeat("a", values.MaxEmailLength) + "@legacy.test"
	seedHistoricalMemberEmail(t, e, e.member.UserID, address)
	mail := &capturedMail{}
	h := NewHandlers(e.svc).WithPasswordReset(mail).WithPasswordLinkBase("https://crm.example.test")
	sent := make(chan struct{})
	h.resetSendStarted = func() { close(sent) }
	rec := httptest.NewRecorder()
	h.RequestPasswordReset(rec, httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password",
		strings.NewReader(`{"email":"`+strings.ToUpper(address)+`"}`)).WithContext(e.wsOnlyCtx()))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("reset request = %d, want 202: %s", rec.Code, rec.Body)
	}
	<-sent
	if mail.sent != 1 || mail.to != address {
		t.Fatalf("reset mail = %+v, want one message to the stored address", mail)
	}
}

func TestNewMembersCannotBypassTheAddressBound(t *testing.T) {
	for _, tc := range []struct {
		name   string
		invite bool
	}{{"invited", true}, {"former", false}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupRevocationEnv(t, "bounded-new-member")
			ctx := e.wsCtx(e.admin)
			address := strings.Repeat("a", values.MaxEmailLength) + "@legacy.test"
			var err error
			if tc.invite {
				_, _, err = e.svc.InviteUser(ctx, e.admin, InviteUserInput{Email: address, DisplayName: "New member", Role: "rep"})
			} else {
				_, err = e.svc.CreateFormerMember(ctx, e.admin, FormerMemberInput{Email: address, DisplayName: "Former member"})
			}
			fault, ok := errors.AsType[*values.ParseError](err)
			if !ok || fault.Field != "email" || fault.Code != "too_long" || fault.Max != values.MaxEmailLength {
				t.Fatalf("new member = %v, want email/too_long", err)
			}
		})
	}
}

func TestAFormerMemberLookupKeepsALegacyMergedAddress(t *testing.T) {
	e := setupRevocationEnv(t, "legacy-former-email")
	address := strings.Repeat("a", values.MaxEmailLength) + "@legacy.test"
	seedHistoricalMemberEmail(t, e, e.member.UserID, address)
	if _, err := e.owner.Exec(e.wsOnlyCtx(),
		`UPDATE app_user SET merged_into_user_id = @survivor, removed_at = now(), status = 'deactivated' WHERE id = @member`,
		pgx.StrictNamedArgs{"survivor": e.admin.UserID, "member": e.member.UserID}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/users/former",
		strings.NewReader(`{"email":"`+address+`","display_name":"Former colleague"}`))
	req = req.WithContext(withIdentity(e.wsCtx(e.admin), e.admin))
	NewHandlers(e.svc).CreateFormerMember(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("former member lookup = %d, want 200: %s", rec.Code, rec.Body)
	}
	var saved struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID != e.admin.UserID.String() {
		t.Fatalf("former member resolved to %s, want %s", saved.ID, e.admin.UserID)
	}
}

func seedHistoricalMemberEmail(t *testing.T, e *revocationEnv, member ids.UserID, address string) {
	t.Helper()
	if _, err := e.owner.Exec(e.wsOnlyCtx(),
		`UPDATE app_user SET email = @email, email_domain_folded = 'legacy.test' WHERE id = @member`,
		pgx.StrictNamedArgs{"email": address, "member": member}); err != nil {
		t.Fatal(err)
	}
}
