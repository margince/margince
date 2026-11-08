// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// callerCaptureRefusal is capture.CaptureRefusal for the signed-in human. It is
// nil without one, so the connect branch's own 401 answers that request.
func callerCaptureRefusal(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil
	}
	return capture.CaptureRefusal(actor.Permissions, actor.SeatType)
}

// seatCapture is the connect screen's one answer for the caller's role, so it
// hides every connect button instead of offering ones that refuse.
func seatCapture(ctx context.Context) *crmcontracts.SeatCapture {
	refusal, refused := errors.AsType[*capture.RoleCannotCaptureError](callerCaptureRefusal(ctx))
	if !refused {
		return &crmcontracts.SeatCapture{Allowed: true}
	}
	reason := crmcontracts.SeatCaptureReasonRoleCannotCapture
	missing := crmcontracts.SeatCaptureMissing(refusal.Missing)
	return &crmcontracts.SeatCapture{Allowed: false, Reason: &reason, Missing: &missing}
}

// requireCapturingGrantor is liveGrantorAuthority plus capture.CaptureRefusal,
// for a mail or calendar consent: an extension's OAuth app captures nothing.
func (h connectorHandlers) requireCapturingGrantor(ctx context.Context, st connectState) error {
	rbac, seat, err := h.liveGrantorAuthority(ctx, st)
	if err != nil {
		return err
	}
	return capture.CaptureRefusal(rbac.Permissions, seat)
}

// grantorRefusalOutcome is where a consent callback lands when its human may
// not grant: a role that cannot capture has its own copy.
func grantorRefusalOutcome(err error) string {
	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); refused {
		return outcomeRoleCannotCapture
	}
	return outcomeError
}

// storeConnection is the terminal half of a connect with no consent step: seal
// the credential through Registry.Connect and answer with the stored row.
func (h connectorHandlers) storeConnection(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string, auth connector.Auth) {
	if _, err := h.registry.Connect(ctx, provider, auth); err != nil {
		writeConnectRefusal(w, r, provider, err)
		return
	}
	views, err := h.registry.Connections(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "connector: reading back connection", "provider", provider, "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   codeConnectorStoreFailed,
			Detail: "The connection was stored but could not be read back.",
		})
		return
	}
	for _, v := range views {
		if v.Provider == provider {
			conn := toContractConnection(v)
			httperr.WriteJSON(w, http.StatusOK, crmcontracts.ConnectConnectorResponse{Connection: &conn})
			return
		}
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusInternalServerError,
		Code:   codeConnectorStoreFailed,
		Detail: "The connection was stored but did not appear in the read-back.",
	})
}

// writeConnectRefusal answers a Registry.Connect that stored nothing. A role
// that cannot capture is the caller's to hear about, never a 500.
func writeConnectRefusal(w http.ResponseWriter, r *http.Request, provider string, err error) {
	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); refused {
		httperr.Write(w, r, err)
		return
	}
	if errors.Is(err, apperrors.ErrScopeExceeded) {
		// The transports grant the descriptor's scopes from the human's own
		// authority, so a human cannot normally reach this.
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusForbidden,
			Code:   "scope_exceeded",
			Detail: "This mailbox connection requires a capture scope that was not granted.",
		})
		return
	}
	slog.ErrorContext(r.Context(), "connector: persisting connection", "provider", provider, "err", err)
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusInternalServerError,
		Code:   codeConnectorStoreFailed,
		Detail: "The connection could not be stored. Nothing was captured; try again.",
	})
}
