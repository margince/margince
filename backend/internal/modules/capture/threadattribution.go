// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReconcileThreadAttributionSeatTx retries one capturing seat's attribution
// after publication, under that seat's current authority rather than the worker's.
func (s *Sink) ReconcileThreadAttributionSeatTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID, authority PartyAuthority) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	rbac, seatType, err := authority.EffectiveAuthorityTx(ctx, tx, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	acting := connectorAuthorityContext(ctx, "thread_repair", ids.From[ids.UserKind](seat), rbac, seatType)
	if err := auth.EnsureActivityContentVisibleLive(acting, tx, id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	var fields ActivityFields
	var key string
	err = tx.QueryRow(ctx, `SELECT a.kind,coalesce(a.subject,''),coalesce(a.thread_key,'') FROM activity a
 WHERE a.id=@id AND a.archived_at IS NULL AND a.restricted_at IS NULL AND activity_thread_available(a.thread_key)
 AND EXISTS(SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id AND ci.user_id=@seat)`, pgx.NamedArgs{"id": id, threadArgSeat: seat}).Scan(&fields.Kind, &fields.Subject, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rec := connector.NormalizedRecord{Fields: fields, ThreadKey: key}
	if err := s.reconcileThreadProjectTx(acting, tx, id, rec); err != nil {
		return err
	}
	if s.deals.Candidates == nil || s.deals.Filer == nil {
		return nil
	}
	_, err = fileOnDeal(dealFiling{
		decider: acting, writer: ctx, filer: s.deals.Filer,
		evidence: map[string]any{fieldReason: "thread_boundary_publication", "decided_as": seat},
	}, tx, id, s.deals.Candidates, nil)
	return err
}

func (s *Sink) reconcileThreadProjectTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	if s.projectKeys == nil || s.stampProject == nil {
		return nil
	}
	filed, err := alreadyFiledUnderAProject(ctx, tx, id)
	if err != nil {
		return err
	}
	var project ids.UUID
	if !filed {
		project, err = s.decideProject(ctx, tx, rec, id)
		if err != nil || project.IsZero() {
			return err
		}
	}
	return linkActivityToProject(ctx, tx, id, project, s.stampProject)
}
