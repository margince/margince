// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rosterLastActive reads the roster through the handler as caller and answers
// each listed member's last_active_at as the wire carried it.
func rosterLastActive(t *testing.T, e *revocationEnv, caller Identity) map[ids.UUID]*time.Time {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users?include_inactive=true", nil).
		WithContext(withIdentity(e.wsCtx(caller), caller))
	inactive, limit := true, 200
	NewHandlers(e.svc).ListUsers(rec, req, crmcontracts.ListUsersParams{IncludeInactive: &inactive, Limit: &limit})
	if rec.Code != http.StatusOK {
		t.Fatalf("roster status = %d: %s", rec.Code, rec.Body)
	}
	var page crmcontracts.UserListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	out := map[ids.UUID]*time.Time{}
	for _, user := range page.Data {
		out[ids.UUID(user.Id)] = user.LastActiveAt
	}
	return out
}

// signedOutAt signs the env's member out of two sessions, the later one active
// at the instant returned, and has another sign-in reap both.
func signedOutAt(t *testing.T, e *revocationEnv) time.Time {
	t.Helper()
	first, latest := e.signIn(t, "first device"), e.signIn(t, "latest device")
	last := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	e.setLastSeen(t, first, last.Add(-time.Hour))
	e.setLastSeen(t, latest, last)
	for _, token := range []string{first, latest} {
		if err := e.svc.Logout(e.wsOnlyCtx(), token); err != nil {
			t.Fatalf("signing out: %v", err)
		}
	}
	if _, _, err := e.svc.Login(e.wsOnlyCtx(), e.admin.Email, bootstrapPassword, noDevice); err != nil {
		t.Fatalf("the sign-in that reaps: %v", err)
	}
	if n := sessionCount(t, e, e.member.UserID); n != 0 {
		t.Fatalf("the member holds %d sessions after the reap, want none", n)
	}
	return last
}

func TestAMemberAdministratorSeesWhenEachMemberWasLastActive(t *testing.T) {
	e := setupRevocationEnv(t, "last-active")
	last := signedOutAt(t, e)
	fresh := e.seat(t, "fresh", "rep")

	got := rosterLastActive(t, e, e.admin)
	if at := got[e.member.UserID.UUID]; at == nil || !at.Equal(last) {
		t.Errorf("a signed-out member's last activity = %v, want %v", at, last)
	}
	if at := got[fresh.UserID.UUID]; at != nil {
		t.Errorf("a member who never signed in reads as active at %v", at)
	}
	if got[e.admin.UserID.UUID] == nil {
		t.Error("the admin's own row carries no last activity though they are signed in")
	}

	row, err := e.svc.GetUser(e.wsCtx(e.admin), e.admin, e.member.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if row.LastActiveAt == nil || !row.LastActiveAt.Equal(last) {
		t.Errorf("the single-member read's last activity = %v, want %v", row.LastActiveAt, last)
	}
}

func TestAColleagueWhoMayNotManageMembersNeverSeesTheirActivity(t *testing.T) {
	e := setupRevocationEnv(t, "last-active-rep")
	signedOutAt(t, e)

	got := rosterLastActive(t, e, e.member)
	if len(got) < 2 {
		t.Fatalf("the roster lists %d members, want the admin and the member", len(got))
	}
	for id, at := range got {
		if at != nil {
			t.Errorf("a caller without user_admin read %s's last activity", id)
		}
	}
}

// A delegated administrator sees what ListUserSessions would show them: a
// member they outrank, and never an admin.
func TestADelegatedAdministratorSeesNoAdminsActivity(t *testing.T) {
	e := setupRevocationEnv(t, "last-active-delegate")
	last := signedOutAt(t, e)
	delegate := e.seat(t, "delegate", e.customRole(t, "Member admin", "rep",
		map[string]storedGrant{objectUserAdmin: fullGrant}))

	got := rosterLastActive(t, e, delegate)
	if at := got[e.member.UserID.UUID]; at == nil || !at.Equal(last) {
		t.Errorf("a member the delegate outranks reads as last active at %v, want %v", at, last)
	}
	if at := got[e.admin.UserID.UUID]; at != nil {
		t.Errorf("the delegate read an admin's last activity on the roster: %v", at)
	}
	row, err := e.svc.GetUser(e.wsCtx(delegate), delegate, e.admin.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if row.LastActiveAt != nil {
		t.Errorf("the delegate read an admin's last activity on the single-member read: %v", row.LastActiveAt)
	}
}

// A delegate with a write verb and no read gets the member without the activity, as
// ListUserSessions refuses them. Adding read shows it.
func TestAWriteOnlyDelegateNeverSeesAMembersActivity(t *testing.T) {
	e := setupRevocationEnv(t, "last-active-write-only")
	last := signedOutAt(t, e)
	deleter := deleteOnlyMemberAdmin(t, e)

	row, err := e.svc.GetUser(e.wsCtx(deleter), deleter, e.member.UserID)
	if err != nil {
		t.Fatalf("the write-only delegate's single-member read: %v", err)
	}
	if row.LastActiveAt != nil {
		t.Errorf("a delegate without user_admin read saw the member's last activity: %v", row.LastActiveAt)
	}

	reader := deleter
	reader.Permissions.Objects = map[string]principal.ObjectGrant{objectUserAdmin: {Read: true, Delete: true}}
	row, err = e.svc.GetUser(e.wsCtx(reader), reader, e.member.UserID)
	if err != nil {
		t.Fatalf("the reading delegate's single-member read: %v", err)
	}
	if row.LastActiveAt == nil || !row.LastActiveAt.Equal(last) {
		t.Errorf("adding read hid the activity: got %v, want %v", row.LastActiveAt, last)
	}
}
