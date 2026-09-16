// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithMFAChallengeSigner injects the signer for the 202 login challenge and its
// /auth/mfa completion. Unset leaves the challenge path unavailable, which a
// login that would challenge treats as fail-closed.
func (h Handlers) WithMFAChallengeSigner(signer MFAChallengeSigner) Handlers {
	h.mfaSigner = signer
	return h
}

// writeMFAChallenge mints a signed, short-lived challenge for the member and
// answers 202. Fails closed with no signer wired — an unsigned challenge is no
// challenge at all.
func (h Handlers) writeMFAChallenge(w http.ResponseWriter, r *http.Request, userID ids.UserID) {
	if h.mfaSigner == nil {
		httperr.Write(w, r, errors.New("identity: no signer is configured for the second-factor challenge"))
		return
	}
	challenge, err := h.mfaSigner.Sign(userID.String(), mfaChallengeTTL)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.MfaChallenge{MfaChallenge: challenge})
}

// CompleteMfaChallenge implements POST /auth/mfa: verify the challenge and the
// second factor, and open a session. Every refusal — a tampered or expired
// challenge, a wrong code, a replayed challenge, an unparseable subject —
// answers the same neutral 401, so the endpoint discloses nothing about which
// half was wrong.
//
// The throttles mirror the login pair's, because this is the login endpoint's
// second half and guards the same account: a per-IP attempt cap ahead of any
// work, and a per-user failure counter keyed by the user id the VERIFIED
// signature names — after Verify, so a forged token cannot spend a victim's
// budget, and never from the body, which the caller chooses.
func (h Handlers) CompleteMfaChallenge(w http.ResponseWriter, r *http.Request) {
	if h.mfaSigner == nil {
		httperr.Write(w, r, errors.New("identity: no signer is configured for the second-factor challenge"))
		return
	}
	if !h.mfaPerIP.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	var req crmcontracts.MfaLoginRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.MfaChallenge == "" || req.Code == "" {
		httperr.Unauthorized(w, r, "the code, or the challenge, is not valid")
		return
	}
	subject, jti, err := h.mfaSigner.Verify(req.MfaChallenge)
	if err != nil {
		httperr.Unauthorized(w, r, "the code, or the challenge, is not valid")
		return
	}
	userID, err := ids.ParseAs[ids.UserKind](subject)
	if err != nil {
		httperr.Unauthorized(w, r, "the code, or the challenge, is not valid")
		return
	}
	if h.mfaFailures.Blocked(userID.String()) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	id, token, err := h.svc.CompleteMFAChallenge(withUserAgent(r.Context(), r.UserAgent()), userID, jti, req.Code)
	if err != nil {
		if errors.Is(err, errBadMFACode) || errors.Is(err, errMFANotEnrolled) {
			h.mfaFailures.Record(userID.String())
			httperr.Unauthorized(w, r, "the code, or the challenge, is not valid")
			return
		}
		httperr.Write(w, r, err)
		return
	}
	setSessionCookie(w, token)
	// The same response builder /me and /auth/login use.
	httperr.WriteJSON(w, http.StatusOK, h.meResponse(r.Context(), id))
}
