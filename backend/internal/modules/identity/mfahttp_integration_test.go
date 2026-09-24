// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The self-service MFA endpoints end to end through the handlers: read the
// (empty) state, start a TOTP enrolment, confirm it into an active factor with
// recovery codes, see the state change, and disable it — proving the wiring, the
// otpauth provisioning URI, and the no-store header the one-time secrets need.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func mfaStatus(t *testing.T, h Handlers, e *revocationEnv) crmcontracts.MfaStatus {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetMyMfa(rec, httptest.NewRequest(http.MethodGet, "/v1/me/mfa", nil).WithContext(e.asMember()))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /me/mfa = %d, want 200", rec.Code)
	}
	var status crmcontracts.MfaStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode mfa status: %v", err)
	}
	return status
}

func TestMFAEndpointsEnrolConfirmAndDisable(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-http")
	e.withVault()
	h := NewHandlers(e.svc)
	ctx := e.asMember()

	if s := mfaStatus(t, h, e); s.Enrolled || s.Confirmed {
		t.Fatalf("a fresh member reads as enrolled: %+v", s)
	}

	// Start enrolment: a secret and a scannable URI, marked no-store.
	startRec := httptest.NewRecorder()
	h.StartMyTotpEnrolment(startRec, httptest.NewRequest(http.MethodPost, "/v1/me/mfa/totp", nil).WithContext(ctx))
	if startRec.Code != http.StatusOK {
		t.Fatalf("start enrolment = %d, want 200", startRec.Code)
	}
	if startRec.Header().Get("Cache-Control") != "no-store" {
		t.Error("the one-time secret was returned without Cache-Control: no-store")
	}
	var enrol crmcontracts.TotpEnrolment
	if err := json.Unmarshal(startRec.Body.Bytes(), &enrol); err != nil {
		t.Fatalf("decode enrolment: %v", err)
	}
	if enrol.Secret == "" || !strings.HasPrefix(enrol.OtpauthUri, "otpauth://totp/") {
		t.Fatalf("enrolment response is incomplete: %+v", enrol)
	}

	// Confirm with a live code; recovery codes come back once.
	code, err := totpCodeAt(enrol.Secret, e.svc.now(), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	confirmRec := httptest.NewRecorder()
	h.ConfirmMyTotp(confirmRec, jsonRequest(ctx, t, http.MethodPost, "/v1/me/mfa/totp/confirm",
		crmcontracts.TotpConfirmRequest{Code: code}))
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirm = %d, want 200 (body %s)", confirmRec.Code, confirmRec.Body.String())
	}
	var codes crmcontracts.RecoveryCodes
	if err := json.Unmarshal(confirmRec.Body.Bytes(), &codes); err != nil {
		t.Fatalf("decode recovery codes: %v", err)
	}
	if len(codes.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(codes.RecoveryCodes), recoveryCodeCount)
	}

	if s := mfaStatus(t, h, e); !s.Confirmed || s.RecoveryCodesLeft != recoveryCodeCount {
		t.Fatalf("state after confirm = %+v", s)
	}

	// Disabling is a step-up: a wrong code answers the neutral 401 and leaves
	// the factor in place — a session alone must not strip it.
	badRec := httptest.NewRecorder()
	h.DisableMyMfa(badRec, jsonRequest(ctx, t, http.MethodDelete, "/v1/me/mfa",
		crmcontracts.MfaDisableRequest{Code: "000000"}))
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("disable with a wrong code = %d, want 401 (body %s)", badRec.Code, badRec.Body.String())
	}
	if !mfaStatus(t, h, e).Enrolled {
		t.Fatal("a refused disable still removed the factor")
	}

	// With a live proof of the factor — here a recovery code — disable clears it.
	delRec := httptest.NewRecorder()
	h.DisableMyMfa(delRec, jsonRequest(ctx, t, http.MethodDelete, "/v1/me/mfa",
		crmcontracts.MfaDisableRequest{Code: codes.RecoveryCodes[0]}))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("disable = %d, want 204 (body %s)", delRec.Code, delRec.Body.String())
	}
	if mfaStatus(t, h, e).Enrolled {
		t.Error("MFA still reads as enrolled after disable over HTTP")
	}
}

// The contract's codeless cases: with no confirmed factor there is nothing the
// step-up would prove, so an empty code disables a merely PENDING enrolment and
// no-ops on an account with nothing enrolled — the code is demanded only where
// a factor actually guards the account.
func TestMFADisableNeedsNoCodeWhereNoConfirmedFactorGuards(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-http-codeless")
	e.withVault()
	h := NewHandlers(e.svc)
	ctx := e.asMember()

	noneRec := httptest.NewRecorder()
	h.DisableMyMfa(noneRec, jsonRequest(ctx, t, http.MethodDelete, "/v1/me/mfa",
		crmcontracts.MfaDisableRequest{}))
	if noneRec.Code != http.StatusNoContent {
		t.Fatalf("codeless disable with nothing enrolled = %d, want the no-op 204 (body %s)",
			noneRec.Code, noneRec.Body.String())
	}

	startRec := httptest.NewRecorder()
	h.StartMyTotpEnrolment(startRec, httptest.NewRequest(http.MethodPost, "/v1/me/mfa/totp", nil).WithContext(ctx))
	if startRec.Code != http.StatusOK {
		t.Fatalf("start enrolment = %d, want 200", startRec.Code)
	}
	pendingRec := httptest.NewRecorder()
	h.DisableMyMfa(pendingRec, jsonRequest(ctx, t, http.MethodDelete, "/v1/me/mfa",
		crmcontracts.MfaDisableRequest{}))
	if pendingRec.Code != http.StatusNoContent {
		t.Fatalf("codeless disable of a pending enrolment = %d, want 204 (body %s)",
			pendingRec.Code, pendingRec.Body.String())
	}
	if mfaStatus(t, h, e).Enrolled {
		t.Error("the pending enrolment survived a codeless disable")
	}
}

// jsonRequest builds a request carrying the marshalled body on the given context.
func jsonRequest[T any](ctx context.Context, t *testing.T, method, path string, payload T) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}
