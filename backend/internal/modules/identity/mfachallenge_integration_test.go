// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The MFA login challenge end to end: a member with a confirmed factor gets no
// session from the password alone (a 202 challenge), and completing the
// challenge with a code opens the session. A wrong code is refused, and a member
// with no factor still signs in on the password alone.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// stubMFASigner is a deterministic stand-in for the composition root's HMAC
// signer: it binds the subject with no clock, so a test drives the flow without
// a real key or wall-clock expiry.
type stubMFASigner struct{}

func (stubMFASigner) Sign(userID string, _ time.Duration) string { return "stub." + userID }

func (stubMFASigner) Verify(token string) (string, error) {
	if sub, ok := strings.CutPrefix(token, "stub."); ok {
		return sub, nil
	}
	return "", errors.New("stub: bad challenge")
}

func (e *revocationEnv) enrolMemberMFA(t *testing.T) string {
	t.Helper()
	ctx := e.asMember()
	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start enrolment: %v", err)
	}
	code, err := totpCodeAt(secret, e.svc.now(), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ConfirmTOTP(ctx, code); err != nil {
		t.Fatalf("confirm enrolment: %v", err)
	}
	return secret
}

func TestLoginWithAConfirmedFactorRequiresTheSecondStep(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-login-svc")
	e.withVault()
	secret := e.enrolMemberMFA(t)

	// The password alone no longer completes: errMFARequired, no token, but the
	// identity carries the user the challenge must bind to.
	id, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if !errors.Is(err, errMFARequired) {
		t.Fatalf("login of an MFA member: err=%v, want errMFARequired", err)
	}
	if token != "" {
		t.Error("a session was minted before the second factor")
	}
	if id.UserID != e.member.UserID {
		t.Error("the challenge identity is not the member who signed in")
	}

	// Completing with a live code mints the session.
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	_, sessionToken, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, fresh)
	if err != nil {
		t.Fatalf("CompleteMFAChallenge: %v", err)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), sessionToken); err != nil {
		t.Fatalf("the session minted after the second factor does not authenticate: %v", err)
	}

	// A wrong code is refused.
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, "000000"); !errors.Is(err, errBadMFACode) {
		t.Fatalf("wrong second-factor code: err=%v, want errBadMFACode", err)
	}
}

func TestLoginWithoutAFactorStillSignsInOnThePassword(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-none")
	e.withVault()
	// No enrolment: the member signs in on the password alone.
	_, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if err != nil || token == "" {
		t.Fatalf("password login for a member with no factor: err=%v token=%q", err, token)
	}
}

func TestMFALoginChallengeOverHTTP(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-login-http")
	e.withVault()
	secret := e.enrolMemberMFA(t)
	h := NewHandlers(e.svc).WithMFAChallengeSigner(stubMFASigner{})

	// Password login returns 202 with a challenge, no session cookie.
	loginBody := []byte(`{"email":"` + e.member.Email + `","password":"` + memberPassword + `"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(loginBody)).WithContext(e.wsOnlyCtx())
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	h.Login(loginRec, loginReq)
	if loginRec.Code != http.StatusAccepted {
		t.Fatalf("password login of an MFA member = %d, want 202 (body %s)", loginRec.Code, loginRec.Body.String())
	}
	if hasSessionCookie(loginRec) {
		t.Error("a session cookie was set before the second factor")
	}
	var challenge crmcontracts.MfaChallenge
	if err := json.Unmarshal(loginRec.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if challenge.MfaChallenge == "" {
		t.Fatal("the 202 carried no challenge")
	}

	// Completing at /auth/mfa opens the session.
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	mfaBody, err := json.Marshal(crmcontracts.MfaLoginRequest{MfaChallenge: challenge.MfaChallenge, Code: fresh})
	if err != nil {
		t.Fatal(err)
	}
	mfaReq := httptest.NewRequest(http.MethodPost, "/v1/auth/mfa", bytes.NewReader(mfaBody)).WithContext(e.wsOnlyCtx())
	mfaReq.Header.Set("Content-Type", "application/json")
	mfaRec := httptest.NewRecorder()
	h.CompleteMfaChallenge(mfaRec, mfaReq)
	if mfaRec.Code != http.StatusOK {
		t.Fatalf("complete mfa = %d, want 200 (body %s)", mfaRec.Code, mfaRec.Body.String())
	}
	if !hasSessionCookie(mfaRec) {
		t.Error("no session cookie after completing the second factor")
	}
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}
