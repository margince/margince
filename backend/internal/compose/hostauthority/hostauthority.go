// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package hostauthority lets a door that runs as the system act as the host it
// serves, with the authority the host's seat holds now.
package hostauthority

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Context acts as the host, with the authority their seat holds now.
func Context(ctx context.Context, seats *identity.Service, host ids.UserID) (context.Context, error) {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	rbac, seat, err := seats.EffectiveAuthority(ctx, workspace, host.UUID)
	if err != nil {
		return nil, err
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + host.String(), UserID: host.UUID, SeatType: seat, Permissions: rbac.Permissions, TeamIDs: rbac.TeamIDs}), nil
}

// BookerContacts is the contacts seam of the public booking door. The door
// runs as the system, so the gap fill acts with the host's own authority. An
// anonymous booker can then never change a record the host may not.
type BookerContacts struct {
	Store *contacts.Store
	Seats *identity.Service
}

func (b BookerContacts) EnsureBookerByEmail(ctx context.Context, host ids.UserID, fullName, email, source string, erasure storekit.ErasureAdmission) (ids.UUID, ids.LeadID, bool, error) {
	return b.Store.EnsureBookerByEmail(ctx, host, fullName, email, source, erasure)
}

func (b BookerContacts) FillBookerGaps(ctx context.Context, contact ids.UUID, host ids.UserID, source string) error {
	hostCtx, err := Context(ctx, b.Seats, host)
	if err != nil {
		return err
	}
	return b.Store.FillBookerGaps(hostCtx, contact, host, source)
}
