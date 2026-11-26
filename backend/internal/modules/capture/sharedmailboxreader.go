// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ForeignMailboxError refuses a shared mailbox outside the installation's own
// domains. A delegated service account reads any mailbox of its domain, so
// the domain must be one a human vouched for.
type ForeignMailboxError struct{ Address string }

func (e *ForeignMailboxError) Error() string {
	return fmt.Sprintf("%s is not on one of this installation's own mail domains; add the domain under own domains first", e.Address)
}

func (e *ForeignMailboxError) Unwrap() error { return apperrors.ErrInvalidArgument }

// RefuseForeignSharedMailbox refuses an address whose domain is not one of the
// trusted own domains, the set that decides what capture keeps.
func (r *Registry) RefuseForeignSharedMailbox(ctx context.Context, address string) error {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return err
	}
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		own, err := TrustedOwnDomainsTx(ctx, tx)
		if err != nil {
			return err
		}
		if !own.AllInternal([]string{bareAddress(address)}) {
			return &ForeignMailboxError{Address: bareAddress(address)}
		}
		return nil
	})
}

// probeShared checks a shared bundle against its provider, for a connector
// whose shared credential differs from the personal one. The bundle it
// returns is the one sealed.
func probeShared(ctx context.Context, c connector.Connector, auth connector.Auth) (connector.Auth, error) {
	reader, ok := c.(connector.SharedMailboxReader)
	if !ok {
		return auth, nil
	}
	return reader.ProbeShared(ctx, auth)
}

// runCredential resolves a connection's credential and the connector that
// runs it. A shared row of a provider with its own shared grant opens that
// provider's shared reader; every other row runs the registered connector.
//
//nolint:ireturn // the connector port's own seam; each provider is a different struct behind it
func (r *Registry) runCredential(
	ctx context.Context, c connector.Connector, grantedBy *ids.UserID, credentialRef *string, authBytes []byte,
) (connector.Connector, connector.Auth, error) {
	auth, err := r.syncCredential(ctx, credentialRef, authBytes)
	if err != nil {
		return nil, nil, err
	}
	reader, ok := c.(connector.SharedMailboxReader)
	if grantedBy != nil || !ok {
		return c, auth, nil
	}
	return reader.OpenShared(auth)
}
