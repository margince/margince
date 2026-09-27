// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Who destroys the provider secret a departure left behind.
//
// The WITHDRAWAL half is here (capturewithdrawal.go) and has to be, because it
// must commit with the deactivation. The DESTRUCTION half is not: the secret
// lives in a vault capture owns and identity knows nothing about, and the call
// that deletes it is a request to an external store that no transaction can
// hold. So the composition root injects the answer, the way it does for the
// licensed seat ceiling and the anchor company.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CaptureCredentialReaper destroys the vaulted credentials of a seat whose
// capture connections have already been withdrawn.
//
// It reports NO error, deliberately. It runs after the deactivation has
// committed, and the deactivation is the thing the operator asked for and
// the thing that stopped the mail. Failing their request because a cleanup
// afterwards did not land would report a departure that did not happen, and
// invite a retry of a deactivation that is already idempotently done. The
// implementation logs its own failures, and what it leaves behind is a
// disconnected row still naming a secret — which is the same state a crash
// between capture's own phases leaves, and which capture's sweep finishes.
type CaptureCredentialReaper func(ctx context.Context, userID ids.UserID)

// WithCaptureCredentialReaper binds the destruction half of a departure's
// withdrawal, wired once at composition because capture owns the vault.
//
// Nil ⟹ nothing is reaped here: the connections are still withdrawn and the
// mailbox still stops being read, and the secret waits for capture's own
// sweep. A role composed without capture has no vault to reach anyway.
//
// It binds on the SERVICE rather than on the returned handlers, for the reason
// the seat ceiling does: the deactivation writer is not the handler, and one
// installation has one vault whichever surface reaches the writer.
func (h Handlers) WithCaptureCredentialReaper(reap CaptureCredentialReaper) Handlers {
	if h.svc != nil {
		h.svc.captureReaper = reap
	}
	return h
}
