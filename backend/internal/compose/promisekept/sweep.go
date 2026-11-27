// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promisekept runs the sweep that closes a promise to invite someone
// once an accepted later meeting shows it kept. It reopens a close whose
// evidence is gone.
package promisekept

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// Sweep checks every promise that has a responsible seat against the meetings
// that seat may read.
type Sweep struct {
	Pool  *pgxpool.Pool
	Store *contacts.Store
	// AskedOf names the one colleague a promise belongs to: the seat the claim
	// names, else the owner of shared-mailbox mail. Zero means nobody.
	AskedOf func(ctx context.Context, tx pgx.Tx, source ids.UUID, seat *ids.UUID) (ids.UUID, error)
	// Rbac is the RBAC vocabulary an owner's grants resolve under.
	Rbac identity.RbacObjects
}

// Workspace runs one workspace's pass. The writes are the system's. Each promise
// is decided in its own transaction under its owner's authority in that
// transaction, so a meeting the owner cannot open never closes it.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	writer := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), contacts.PromiseKeptActor)
	var claims []ids.UUID
	if err := database.WithWorkspaceTx(writer, s.Pool, func(tx pgx.Tx) error {
		var err error
		claims, err = s.Store.KeptPromiseCandidatesTx(writer, tx)
		return err
	}); err != nil {
		return err
	}
	var failures []error
	for _, claim := range claims {
		if err := database.WithWorkspaceTx(writer, s.Pool, func(tx pgx.Tx) error {
			// Authority is taken before any row lock, as authorization writers order it.
			if err := identity.HoldAuthorizationTx(writer, tx); err != nil {
				return err
			}
			return s.Store.ReconcileKeptPromiseTx(writer, tx, claim, s)
		}); err != nil {
			failures = append(failures, fmt.Errorf("promise %s: %w", claim, err))
		}
	}
	return errors.Join(failures...)
}

// Owner implements contacts.KeptSeats.
func (s Sweep) Owner(ctx context.Context, tx pgx.Tx, source ids.UUID, named *ids.UUID) (ids.UUID, error) {
	return s.AskedOf(ctx, tx, source, named)
}

// Reader implements contacts.KeptSeats. A seat that is gone holds no grants, so
// nothing it could read shows a promise kept.
func (s Sweep) Reader(ctx context.Context, tx pgx.Tx, owner ids.UUID) (context.Context, error) {
	reader := principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + owner.String(), UserID: owner}
	rbac, seat, err := identity.EffectiveAuthorityTx(ctx, tx, s.Rbac, owner)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
	case err != nil:
		return nil, fmt.Errorf("read the authority of promise owner %s: %w", owner, err)
	default:
		reader = authz.HumanPrincipal(owner, rbac, seat)
	}
	return principal.WithActor(ctx, reader), nil
}
