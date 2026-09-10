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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
// the installation's workspace bound, and that person as the acting principal —
// which is where the self-scoped session calls read the caller from.
func (e *revocationEnv) asMember() context.Context {
	return withHumanPrincipal(e.wsOnlyCtx(), e.member)
}
func (e *revocationEnv) asAdmin() context.Context { return withHumanPrincipal(e.wsOnlyCtx(), e.admin) }

func TestListSessionsReturnsLiveSessionsMarkingTheCurrentOne(t *testing.T) {
	e := setupRevocationEnv(t, "sessions-list")
	ctx := e.asMember()

	current := e.signIn(t, "Mozilla/5.0 (current device)")
	e.signIn(t, "Mozilla/5.0 (other device)")

	sessions, err := e.svc.ListSessions(ctx, hashToken(current))
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(sessions))
	}

	var currentCount int
	for _, s := range sessions {
		if s.SignedInAt.IsZero() || s.LastActiveAt.IsZero() {
			t.Errorf("session %s: timestamps must be populated, got signed_in=%v last_active=%v",
				s.ID, s.SignedInAt, s.LastActiveAt)
		}
		if s.UserAgent == nil {
			t.Errorf("session %s: user-agent captured at login must be reported", s.ID)
		}
		if s.Current {
			currentCount++
			if s.UserAgent == nil || *s.UserAgent != "Mozilla/5.0 (current device)" {
				t.Errorf("current session carries the wrong device: %v", s.UserAgent)
			}
		}
	}
	if currentCount != 1 {
		t.Errorf("exactly one session must be marked current, got %d", currentCount)
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
	strangerRec := httptest.NewRecorder()
	strangerReq := httptest.NewRequest(http.MethodDelete, "/v1/me/sessions/x", nil).WithContext(ctx)
	h.RevokeMySession(strangerRec, strangerReq, openapi_types.UUID(ids.NewV7()))
	if strangerRec.Code != http.StatusNotFound {
		t.Fatalf("revoking a stranger's session id = %d, want 404", strangerRec.Code)
	}
}

// idOf finds the session opened under the given user-agent. A test that wants to
// act on a specific session names it by the device it recorded, rather than
// assuming an order the query never promised.
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
