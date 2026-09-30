// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam between a departure and the vault.
//
// Deactivating a seat withdraws their capture connections in identity's own
// transaction, because "we have stopped reading their mail" must not lag "this
// colleague has left". The secret those connections named is a different
// problem: it lives in a vault capture owns, and deleting it is a call to an
// external store no transaction can hold. Neither module may import the other,
// so the wiring tier hands identity the destruction half as a function.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// installCaptureCredentialReaper binds the vault half of a departure onto the
// Server's identity handlers.
//
// Wired HERE, after the options have run, for the reason the tool registry is:
// several options install a registry and the last one wins, so binding at any
// single option's site would hand identity a registry the Server does not end
// up serving. A role that composed no capture registry binds nothing, and
// deactivation still withdraws the connections — it simply leaves the secret
// for capture's own sweep, which is the same place a failure here leaves it.
func installCaptureCredentialReaper(s *Server, log *slog.Logger) {
	registry := s.connectorHandlers.registry
	if registry == nil {
		return
	}
	s.authHandlers = s.WithCaptureCredentialReaper(reapWithdrawnCredentials(registry, log))
}

// reapWithdrawnCredentials adapts capture's reap onto identity's port.
//
// It swallows nothing: the error is logged here, where there is a logger, and
// deliberately not returned, because the port reports none. The departure has
// already committed and is the thing the operator asked for — failing it
// because a cleanup afterwards did not land would report a deactivation that
// did not happen, and invite a retry of one that is already done. What a
// failure leaves is a disconnected row still naming a secret, which is the
// same state a crash between capture's own withdrawal phases leaves.
func reapWithdrawnCredentials(registry *capture.Registry, log *slog.Logger) identity.CaptureCredentialReaper {
	return func(ctx context.Context, userID ids.UserID) {
		if err := registry.ReapWithdrawnCredentials(ctx, userID); err != nil {
			log.ErrorContext(ctx, "capture: a deactivated seat's vaulted credentials were not destroyed",
				"user_id", userID, "error", err)
		}
	}
}
