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
)

// Sweep checks every promise that has a responsible seat against the meetings
// that seat may read.
type Sweep struct {
	Pool  *pgxpool.Pool
	Store *contacts.Store
}

// Workspace runs one workspace's pass. The writes are the system's. What counts
// as evidence is decided under each owner's own authority, so a meeting the
// owner cannot open never closes their promise.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithWorkspaceID(ctx, workspace)
	writer := principal.SystemActing(ctx, contacts.PromiseKeptActor)
	var candidates []contacts.KeptCandidate
	if err := database.WithWorkspaceTx(writer, s.Pool, func(tx pgx.Tx) error {
		var err error
		candidates, err = s.Store.KeptPromiseCandidatesTx(writer, tx)
		return err
	}); err != nil {
		return err
	}
	byOwner := map[ids.UUID][]contacts.KeptCandidate{}
	for _, candidate := range candidates {
		byOwner[candidate.Owner] = append(byOwner[candidate.Owner], candidate)
	}
	users := identity.NewService(s.Pool)
	var failures []error
	for owner, claims := range byOwner {
		if err := s.reconcileOwner(ctx, writer, users, workspace, owner, claims); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s Sweep) reconcileOwner(
	ctx, writer context.Context, users *identity.Service, workspace, owner ids.UUID, claims []contacts.KeptCandidate,
) error {
	reader := principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + owner.String(), UserID: owner}
	rbac, seat, err := users.EffectiveAuthority(ctx, workspace, owner)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		// A seat that is gone holds no grants, so nothing it could read shows a promise kept.
	case err != nil:
		return fmt.Errorf("read the authority of promise owner %s: %w", owner, err)
	default:
		reader.SeatType, reader.TeamIDs, reader.Permissions = seat, rbac.TeamIDs, rbac.Permissions
	}
	readerCtx := principal.WithActor(ctx, reader)
	return database.WithWorkspaceTx(writer, s.Pool, func(tx pgx.Tx) error {
		return s.Store.ReconcileKeptPromisesTx(writer, readerCtx, tx, claims)
	})
}
