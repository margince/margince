// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// SharedMailboxReader is a connector that reads a shared mailbox with an
// installation-wide grant: an app permission, or a service account under
// domain-wide delegation. Its sealed bundle is not the personal one, so the
// registry opens the connector a run uses from it.
type SharedMailboxReader interface {
	// ProbeShared checks that the bundle reaches its mailbox, and returns
	// the bundle to seal. A refusal is a *SharedMailboxRefusal.
	ProbeShared(ctx context.Context, auth Auth) (Auth, error)
	// OpenShared returns the connector and the auth one run of the shared
	// mailbox uses.
	OpenShared(auth Auth) (Connector, Auth, error)
}

// The classes a shared-mailbox probe is refused with. Each names what the
// admin fixes, and none carries the provider's own text.
const (
	SharedCredentialsInvalid = "credentials_invalid"
	SharedGrantRefused       = "grant_refused"
	SharedMailboxDenied      = "mailbox_denied"
	SharedMailboxMissing     = "mailbox_not_found"
)

// SharedMailboxRefusal is a shared-mailbox probe the provider turned away.
type SharedMailboxRefusal struct {
	Reason string
	Err    error
}

func (e *SharedMailboxRefusal) Error() string {
	return fmt.Sprintf("shared mailbox refused (%s): %v", e.Reason, e.Err)
}

func (e *SharedMailboxRefusal) Unwrap() error { return e.Err }

// SharedGrantFault classifies a fault of the token step of a probe. A
// refused grant is the admin's to fix; the rest passes through unchanged.
func SharedGrantFault(err error) error {
	if errors.Is(err, ErrAuthRejected) {
		return &SharedMailboxRefusal{Reason: SharedGrantRefused, Err: err}
	}
	return err
}

// SharedMailboxFault classifies a fault of the mailbox step of a probe. The
// token was issued, so a refusal here means the grant does not cover this
// mailbox, or the mailbox does not exist.
func SharedMailboxFault(err error) error {
	var provider *ProviderError
	if errors.As(err, &provider) && provider.Status == http.StatusNotFound {
		return &SharedMailboxRefusal{Reason: SharedMailboxMissing, Err: err}
	}
	if errors.Is(err, ErrAuthRejected) {
		return &SharedMailboxRefusal{Reason: SharedMailboxDenied, Err: err}
	}
	return err
}
