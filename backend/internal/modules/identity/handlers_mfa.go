// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"net/http"
	"net/url"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// The mounted addresses of the routes a member confined by require-MFA may still
// reach: read their state and enrol a factor — never disable one, so a required
// factor cannot be removed from inside the confinement.
const (
	mfaStatusPath      = httpserver.BaseURL + "/me/mfa"
	mfaTotpPath        = httpserver.BaseURL + "/me/mfa/totp"
	mfaTotpConfirmPath = httpserver.BaseURL + "/me/mfa/totp/confirm"
)

// isMFAEnrolRequest reports the routes an un-enrolled member may reach while the
// installation requires a factor. The read-seat ceiling admits the enrolment
// POSTs through it too — enrolling a mandated factor is self-management, not a
// business write.
func isMFAEnrolRequest(r *http.Request) bool {
	switch r.URL.Path {
	case mfaStatusPath:
		return r.Method == http.MethodGet
	case mfaTotpPath, mfaTotpConfirmPath:
		return r.Method == http.MethodPost
	}
	return false
}

// mfaEnrolmentRequiredRefusal is the answer every admission door gives a member
// the installation requires a factor from until they enrol one. One spelling,
// like forcedRotationRefusal, so a client branches on a single code.
func mfaEnrolmentRequiredRefusal() *httperr.DetailedError {
	return &httperr.DetailedError{
		Status: http.StatusForbidden, Code: "mfa_enrolment_required",
		Detail: "this installation requires multi-factor authentication; enrol an authenticator to continue",
	}
}

// GetMyMfa implements GET /me/mfa — the caller's own second-factor state.
func (h Handlers) GetMyMfa(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.MFAEnrolment(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.MfaStatus{
		Enrolled:          status.Enrolled,
		Confirmed:         status.Confirmed,
		RecoveryCodesLeft: status.RecoveryCodesLeft,
	})
}

// DisableMyMfa implements DELETE /me/mfa.
func (h Handlers) DisableMyMfa(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DisableMFA(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StartMyTotpEnrolment implements POST /me/mfa/totp — mint a pending enrolment
// and return its secret and provisioning URI once.
func (h Handlers) StartMyTotpEnrolment(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "not signed in")
		return
	}
	secret, err := h.svc.StartTOTPEnrolment(r.Context())
	if err != nil {
		httperr.Write(w, r, conflictIf(err, errMFAAlreadyEnrolled, "mfa_already_enrolled",
			"a confirmed authenticator is already enrolled; disable it before enrolling another"))
		return
	}
	// Shown once and never retrievable: keep it out of any shared cache.
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.TotpEnrolment{
		Secret:     secret,
		OtpauthUri: otpauthURI(id.WorkspaceName, id.Email, secret),
	})
}

// ConfirmMyTotp implements POST /me/mfa/totp/confirm — verify a code and, on
// success, activate the factor and hand back the one-time recovery codes.
func (h Handlers) ConfirmMyTotp(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.TotpConfirmRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Code == "" {
		httperr.Write(w, r, httperr.Validation("code", "required", "the authenticator code is required"))
		return
	}
	recovery, err := h.svc.ConfirmTOTP(r.Context(), req.Code)
	switch {
	case errors.Is(err, errBadMFACode):
		httperr.Unauthorized(w, r, "the code is not valid")
		return
	case errors.Is(err, errMFAAlreadyEnrolled):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "mfa_already_enrolled",
			Detail: "the factor is already confirmed",
		})
		return
	case errors.Is(err, errMFANotEnrolled):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "mfa_not_enrolled",
			Detail: "start an enrolment before confirming it",
		})
		return
	case err != nil:
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecoveryCodes{RecoveryCodes: recovery})
}

// otpauthURI builds the RFC-KeyURI value an authenticator app scans. The issuer
// falls back to the product name when the installation has not been named, so
// the app still shows a recognizable account rather than a bare colon.
func otpauthURI(issuer, account, secret string) string {
	if issuer == "" {
		issuer = "Margince"
	}
	query := url.Values{
		"secret":    {secret},
		"issuer":    {issuer},
		"algorithm": {"SHA1"},
		"digits":    {"6"},
		"period":    {"30"},
	}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + query.Encode()
}
