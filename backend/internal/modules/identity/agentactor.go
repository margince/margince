// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actingFor is actor for the member routes an agent reaches. It answers a
// human's session Identity, or the live Identity of the colleague whose
// passport an agent holds. Every rank and grant check then reads their roles.
func (h Handlers) actingFor(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	if id, ok := identityFrom(r.Context()); ok {
		return id, true
	}
	id, err := h.svc.agentsUser(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return Identity{}, false
	}
	return id, true
}

// agentsUser is the live Identity of the colleague an agent acts for. A
// passportless agent acts for nobody, and a colleague whose seat is gone has
// nothing to lend.
func (s *Service) agentsUser(ctx context.Context) (Identity, error) {
	p, ok := principal.Actor(ctx)
	if !ok || !p.ActsAsItsUser() {
		return Identity{}, fmt.Errorf("identity: no signed-in member or agent acting for one: %w", apperrors.ErrPermissionDenied)
	}
	wsID, err := s.InstallationWorkspace(ctx)
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		id, err = liveMemberIdentity(ctx, tx, wsID, ids.From[ids.UserKind](p.OnBehalfOf))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, fmt.Errorf("identity: the passport's member is no longer live: %w", apperrors.ErrPermissionDenied)
	}
	return id, err
}
