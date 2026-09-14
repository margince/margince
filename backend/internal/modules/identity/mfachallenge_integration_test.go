// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The MFA login challenge end to end: a member with a confirmed factor gets no
// session from the password alone (a 202 challenge), and completing the
// challenge with a code opens the session. A wrong code is refused, a spent
// challenge is refused, and a member with no factor still signs in on the
// password alone.

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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubMFASigner is a deterministic stand-in for the composition root's HMAC
// signer: it binds the subject and a fresh nonce with no clock, so a test
// drives the flow without a real key or wall-clock expiry. The nonce is a v7 id
// because the spent-challenge table is shared across every test in the
// database, and a fixed nonce would make two tests one replay.
type stubMFASigner struct{}

func (stubMFASigner) Sign(userID string, _ time.Duration) (string, error) {
	return "stub." + userID + "." + ids.NewV7().String(), nil
}

func (stubMFASigner) Verify(token string) (string, string, error) {
	rest, ok := strings.CutPrefix(token, "stub.")
	if !ok {
		return "", "", errors.New("stub: bad challenge")
	}
	sub, jti, ok := strings.Cut(rest, ".")
	if !ok || jti == "" {
		return "", "", errors.New("stub: bad challenge")
	}
	return sub, jti, nil
}

// freshJti mints a challenge nonce for the tests that drive the service
// directly, standing where the handler's verified signature would.
func freshJti() string { return ids.NewV7().String() }

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
	if _, err := e.svc.ConfirmTOTP(ctx, code, ""); err != nil {
		t.Fatalf("confirm enrolment: %v", err)
	}
	return secret
}

func TestLoginWithAConfirmedFactorRequiresTheSecondStep(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-login-svc")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
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

	// Completing with a live code mints the session; the response identity is
	// resolved in the SAME transaction, so it is already the signed-in shape.
	e.atInstant(base.Add(totpStep))
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	signedIn, sessionToken, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, freshJti(), fresh)
	if err != nil {
		t.Fatalf("CompleteMFAChallenge: %v", err)
	}
	if signedIn.Email != e.member.Email || signedIn.UserID != e.member.UserID {
		t.Errorf("the completed challenge resolved identity %q/%s, want the member's own", signedIn.Email, signedIn.UserID)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), sessionToken); err != nil {
		t.Fatalf("the session minted after the second factor does not authenticate: %v", err)
	}

	// A wrong code is refused.
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, freshJti(), "000000"); !errors.Is(err, errBadMFACode) {
		t.Fatalf("wrong second-factor code: err=%v, want errBadMFACode", err)
	}
}

func TestACompletedChallengeCannotBeSpentTwice(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-challenge-replay")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	secret := e.enrolMemberMFA(t)
	jti := freshJti()

	e.atInstant(base.Add(totpStep))
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, jti, code); err != nil {
		t.Fatalf("first completion: %v", err)
	}

	// The same challenge with a NEW valid code: the nonce is spent, so the
	// refusal is the challenge's, not the code's — and it is the same neutral
	// error, disclosing nothing about which half failed.
	e.atInstant(base.Add(2 * totpStep))
	next, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, jti, next); !errors.Is(err, errBadMFACode) {
		t.Fatalf("replayed challenge: err=%v, want errBadMFACode", err)
	}

	// A signer that minted no nonce is refused outright: such a challenge could
	// never be made single-use.
	e.atInstant(base.Add(3 * totpStep))
	last, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, "", last); !errors.Is(err, errBadMFACode) {
		t.Fatalf("nonce-less challenge: err=%v, want errBadMFACode", err)
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
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
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
	e.atInstant(base.Add(totpStep))
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	mfaRec := postMfaCompletion(t, h, e, challenge.MfaChallenge, fresh)
	if mfaRec.Code != http.StatusOK {
		t.Fatalf("complete mfa = %d, want 200 (body %s)", mfaRec.Code, mfaRec.Body.String())
	}
	if !hasSessionCookie(mfaRec) {
		t.Error("no session cookie after completing the second factor")
	}

	// The SAME challenge again, even with a fresh valid code, is spent: the
	// neutral 401 every second-factor miss answers.
	e.atInstant(base.Add(2 * totpStep))
	next, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	replayRec := postMfaCompletion(t, h, e, challenge.MfaChallenge, next)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed challenge over HTTP = %d, want 401 (body %s)", replayRec.Code, replayRec.Body.String())
	}
	if hasSessionCookie(replayRec) {
		t.Error("a replayed challenge still set a session cookie")
	}
}

func TestMfaCompletionFailuresAreRateLimitedPerAccount(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-throttle")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	secret := e.enrolMemberMFA(t)
	h := NewHandlers(e.svc).WithMFAChallengeSigner(stubMFASigner{})

	signer := stubMFASigner{}
	// Ten wrong codes spend the per-account failure budget (the login pair's
	// number), each on its own challenge so the nonce is never what refuses.
	for range 10 {
		challenge, err := signer.Sign(e.member.UserID.String(), mfaChallengeTTL)
		if err != nil {
			t.Fatal(err)
		}
		rec := postMfaCompletion(t, h, e, challenge, "000000")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong code = %d, want 401 (body %s)", rec.Code, rec.Body.String())
		}
	}

	// The eleventh attempt is refused BEFORE any verification — even the right
	// code no longer reaches the factor until the window passes.
	e.atInstant(base.Add(totpStep))
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	challenge, err := signer.Sign(e.member.UserID.String(), mfaChallengeTTL)
	if err != nil {
		t.Fatal(err)
	}
	rec := postMfaCompletion(t, h, e, challenge, fresh)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt over the failure budget = %d, want 429 (body %s)", rec.Code, rec.Body.String())
	}
}

// postMfaCompletion drives POST /auth/mfa with one challenge+code pair.
func postMfaCompletion(t *testing.T, h Handlers, e *revocationEnv, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(crmcontracts.MfaLoginRequest{MfaChallenge: challenge, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/mfa", bytes.NewReader(body)).WithContext(e.wsOnlyCtx())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CompleteMfaChallenge(rec, req)
	return rec
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}
