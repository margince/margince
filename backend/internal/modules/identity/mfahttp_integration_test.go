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

func mfaStatus(t *testing.T, h Handlers, ctx context.Context) crmcontracts.MfaStatus {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetMyMfa(rec, httptest.NewRequest(http.MethodGet, "/v1/me/mfa", nil).WithContext(ctx))
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

	if s := mfaStatus(t, h, ctx); s.Enrolled || s.Confirmed {
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
	h.ConfirmMyTotp(confirmRec, jsonRequest(t, http.MethodPost, "/v1/me/mfa/totp/confirm",
		crmcontracts.TotpConfirmRequest{Code: code}, ctx))
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

	if s := mfaStatus(t, h, ctx); !s.Confirmed || s.RecoveryCodesLeft != recoveryCodeCount {
		t.Fatalf("state after confirm = %+v", s)
	}

	// Disable clears it.
	delRec := httptest.NewRecorder()
	h.DisableMyMfa(delRec, httptest.NewRequest(http.MethodDelete, "/v1/me/mfa", nil).WithContext(ctx))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("disable = %d, want 204", delRec.Code)
	}
	if mfaStatus(t, h, ctx).Enrolled {
		t.Error("MFA still reads as enrolled after disable over HTTP")
	}
}

// jsonRequest builds a POST carrying the marshalled body on the given context.
func jsonRequest(t *testing.T, method, path string, payload crmcontracts.TotpConfirmRequest, ctx context.Context) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}
