// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Self-service session management end to end: a member lists their own live
// sessions (each carrying the device it was opened from and marking the one
// making the request), and ends any one of them — with the same row-scope
// refusal a stranger's session id earns everywhere else in the tree, a 404
// that hides whether it exists at all.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signIn opens one session for the env's member, recording the user-agent the
// login handler would have captured from the request. Returns the raw cookie
// token so the caller can hash it (to name the current session) or authenticate
// with it (to prove a revoke took effect).
func (e *revocationEnv) signIn(t *testing.T, userAgent string) string {
	t.Helper()
	ctx := withUserAgent(e.wsOnlyCtx(), userAgent)
	_, token, err := e.svc.Login(ctx, e.member.Email, memberPassword)
	if err != nil {
		t.Fatalf("member login (%s): %v", userAgent, err)
	}
	return token
}

// asMember and asAdmin present the context admission would build for that human:
// the installation's workspace bound, and that member as the acting principal —
// which is where the self-scoped session calls read the caller from.
func (e *revocationEnv) asMember() context.Context {
	return withHumanPrincipal(e.wsOnlyCtx(), e.member)
}
func (e *revocationEnv) asAdmin() context.Context { return withHumanPrincipal(e.wsOnlyCtx(), e.admin) }

func TestListSessionsReturnsNewestActivityFirstMarkingTheCurrentOne(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-list")
	ctx := e.asMember()

	current := e.signIn(t, "Mozilla/5.0 (current device)")
	other := e.signIn(t, "Mozilla/5.0 (other device)")

	// The list promises newest ACTIVITY first, not newest sign-in: the session
	// opened first is pinned as the more recently active, so a list that in
	// fact ordered by creation could not pass by coincidence.
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	e.setLastSeen(t, current, base.Add(time.Minute))
	e.setLastSeen(t, other, base)

	sessions, err := e.svc.ListSessions(ctx, hashToken(current))
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(sessions))
	}

	// The complete sequence, by position: most recently active first.
	first, second := sessions[0], sessions[1]
	if first.ID == second.ID {
		t.Fatalf("the two logins must be distinct sessions, both %s", first.ID)
	}
	if first.UserAgent == nil || *first.UserAgent != "Mozilla/5.0 (current device)" {
		t.Errorf("sessions[0] must be the most recently active (current device), got %v", first.UserAgent)
	}
	if second.UserAgent == nil || *second.UserAgent != "Mozilla/5.0 (other device)" {
		t.Errorf("sessions[1] must be the least recently active (other device), got %v", second.UserAgent)
	}
	if !first.LastActiveAt.Equal(base.Add(time.Minute)) || !second.LastActiveAt.Equal(base) {
		t.Errorf("last_active_at must report the pinned instants, got %v and %v",
			first.LastActiveAt, second.LastActiveAt)
	}
	if first.SignedInAt.IsZero() || second.SignedInAt.IsZero() {
		t.Errorf("signed_in_at must be populated, got %v and %v", first.SignedInAt, second.SignedInAt)
	}
	if !first.Current || second.Current {
		t.Errorf("only the request's own session is current, got %v and %v", first.Current, second.Current)
	}
}

// setLastSeen pins a session's activity instant directly on its row, named by
// the token it was opened with, so an ordering assertion rests on values the
// test chose rather than on how quickly two logins ran.
func (e *revocationEnv) setLastSeen(t *testing.T, token string, at time.Time) {
	t.Helper()
	tag, err := e.owner.Exec(context.Background(),
		`UPDATE session SET last_seen_at = $1 WHERE token_hash = $2`, at, hashToken(token))
	if err != nil {
		t.Fatalf("pin last_seen_at: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("pin last_seen_at: %d rows updated, want 1", tag.RowsAffected())
	}
}

func TestListSessionsExcludesRevoked(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-live-only")
	ctx := e.asMember()

	current := e.signIn(t, "current")
	e.signIn(t, "stale")

	all, err := e.svc.ListSessions(ctx, hashToken(current))
	if err != nil {
		t.Fatalf("ListSessions before revoke: %v", err)
	}
	staleID := idOf(t, all, "stale")

	if err := e.svc.RevokeSession(ctx, staleID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	after, err := e.svc.ListSessions(ctx, hashToken(current))
	if err != nil {
		t.Fatalf("ListSessions after revoke: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("live session count after revoke = %d, want 1", len(after))
	}
	if after[0].UserAgent == nil || *after[0].UserAgent != "current" {
		t.Errorf("the surviving session is not the one we kept: %v", after[0].UserAgent)
	}
}

func TestRevokeSessionEndsAccessAndIsIdempotent(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-revoke")
	ctx := e.asMember()

	token := e.signIn(t, "device")
	sessions, err := e.svc.ListSessions(ctx, hashToken(token))
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	id := sessions[0].ID

	if err := e.svc.RevokeSession(ctx, id); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := e.svc.Authenticate(ctx, token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("revoked token still authenticates: err=%v", err)
	}

	// A second revoke of the caller's own, already-revoked session is a no-op,
	// not a 404 — the row is theirs, so there is nothing to hide.
	if err := e.svc.RevokeSession(ctx, id); err != nil {
		t.Fatalf("idempotent re-revoke: %v", err)
	}
}

func TestRevokeSessionRefusesAnotherMembersSession(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-scope")

	// The admin's own session, opened through the real login path.
	adminLoginCtx := withUserAgent(e.wsOnlyCtx(), "admin device")
	_, adminToken, err := e.svc.Login(adminLoginCtx, e.admin.Email, bootstrapPassword)
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}
	adminSessions, err := e.svc.ListSessions(e.asAdmin(), hashToken(adminToken))
	if err != nil {
		t.Fatalf("list admin sessions: %v", err)
	}
	adminSessionID := adminSessions[0].ID

	// The member tries to revoke the admin's session: acting as the member, the
	// revoke is scoped to the member's own rows, so the admin's session id is a
	// row-scope miss that reads as not-found — and the admin's session stays live.
	err = e.svc.RevokeSession(e.asMember(), adminSessionID)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-member revoke: err=%v, want ErrNotFound", err)
	}
	if _, err := e.svc.Authenticate(e.asAdmin(), adminToken); err != nil {
		t.Fatalf("admin session must remain live after a refused revoke: %v", err)
	}
}

func TestSessionsOverHTTPMarkCurrentRevokeAndNeverLeakIP(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-http")
	h := NewHandlers(e.svc)
	// The request arrives as admission would leave it: the member's identity and
	// the installation's workspace on the context, and the session cookie whose
	// row the list must mark current.
	token := e.signIn(t, "Mozilla/5.0 (http device)")
	ctx := e.asMember()

	listReq := httptest.NewRequest(http.MethodGet, "/v1/me/sessions", nil).WithContext(ctx)
	listReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	listRec := httptest.NewRecorder()
	h.ListMySessions(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	// The row carries an ip column; the wire must never. Asserted on the raw
	// body, before any typed decode that would drop an unexpected field silently.
	if strings.Contains(listRec.Body.String(), `"ip"`) {
		t.Errorf("session list leaked an ip field: %s", listRec.Body.String())
	}
	var list crmcontracts.MySessionList
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Sessions) != 1 || !list.Sessions[0].Current {
		t.Fatalf("expected one current session, got %+v", list.Sessions)
	}
	sessionID := list.Sessions[0].Id

	delReq := httptest.NewRequest(http.MethodDelete, "/v1/me/sessions/"+sessionID.String(), nil).WithContext(ctx)
	delRec := httptest.NewRecorder()
	h.RevokeMySession(delRec, delReq, sessionID)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", delRec.Code)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("revoked session still authenticates over HTTP: %v", err)
	}

	// A session id that is not the caller's is 404 — never 403, which would
	// confirm it exists.
	strangerID := openapi_types.UUID(ids.NewV7())
	strangerRec := httptest.NewRecorder()
	strangerReq := httptest.NewRequest(http.MethodDelete, "/v1/me/sessions/"+strangerID.String(), nil).WithContext(ctx)
	h.RevokeMySession(strangerRec, strangerReq, strangerID)
	if strangerRec.Code != http.StatusNotFound {
		t.Fatalf("revoking a stranger's session id = %d, want 404", strangerRec.Code)
	}
}

func TestChangePasswordSessionRecordsItsDevice(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-changepw")
	h := NewHandlers(e.svc)

	// A prior session on another device: ChangePassword revokes every session,
	// so the list must end up showing only the one the change itself mints —
	// which is exactly why a blank device on it would erase this account's whole
	// visible history.
	e.signIn(t, "old device")

	// Drive the real handler with the device on the REQUEST: only the handler's
	// own capture carries it onto the minted session. A service call handed a
	// ready-made context would prove the capture, not the wiring that feeds it.
	body, err := json.Marshal(map[string]string{"current_password": memberPassword, "new_password": newMemberPassword})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/change-password", bytes.NewReader(body)).
		WithContext(loginCtx(t, e, e.member.Email, memberPassword))
	req.Header.Set("User-Agent", "Mozilla/5.0 (change-password device)")
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ChangePassword status = %d, want 204", rec.Code)
	}

	var fresh string
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			fresh = c.Value
		}
	}
	if fresh == "" {
		t.Fatal("change-password set no session cookie")
	}

	sessions, err := e.svc.ListSessions(e.asMember(), hashToken(fresh))
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("the fresh session is the only survivor of a password change, got %d", len(sessions))
	}
	if !sessions[0].Current {
		t.Error("the session minted by the change must be marked current")
	}
	if sessions[0].UserAgent == nil || *sessions[0].UserAgent != "Mozilla/5.0 (change-password device)" {
		t.Errorf("the change-password session recorded the wrong device: %v", sessions[0].UserAgent)
	}
}

// idOf finds the session opened under the given user-agent. The list does
// promise newest-activity-first, but two back-to-back logins can share an
// activity instant — so a test that has not pinned last_seen_at names a
// session by its device rather than by position.
func idOf(t *testing.T, sessions []MySession, userAgent string) ids.UUID {
	t.Helper()
	for _, s := range sessions {
		if s.UserAgent != nil && *s.UserAgent == userAgent {
			return s.ID
		}
	}
	t.Fatalf("no session with user-agent %q", userAgent)
	return ids.UUID{}
}
