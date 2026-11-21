// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// partyRecoveryPlace is where the sweep stands: the last pair it offered, and
// the newest activity the sweep in progress reaches. All zero starts a sweep.
type partyRecoveryPlace struct {
	AfterActivity ids.UUID `json:"after_activity"`
	AfterUser     ids.UUID `json:"after_user"`
	Ceiling       ids.UUID `json:"ceiling"`
}

type partyRecoveryTarget struct {
	Activity ids.ActivityID
	User     ids.UUID
}

// RecoverPartyAdmissions revisits failed post-commit work under each owner's live authority.
func (s *Sink) RecoverPartyAdmissions(ctx context.Context, limit int) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if s.ensurer == nil || s.partyAuthority == nil {
		return nil
	}
	var targets []partyRecoveryTarget
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		targets, err = partyRecoveryPageTx(ctx, tx, max(1, min(limit, 200)))
		return err
	}); err != nil {
		return err
	}
	var failed error
	for _, target := range targets {
		acting := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalConnector, ID: connectorPrincipalID("recovery"), UserID: target.User, OnBehalfOf: target.User})
		if err := s.AdmitActivityParties(acting, target.Activity); err != nil {
			failed = errors.Join(failed, fmt.Errorf("recover captured participants: %w", err))
		}
	}
	return failed
}

func partyRecoveryPageTx(ctx context.Context, tx pgx.Tx, limit int) ([]partyRecoveryTarget, error) {
	// The row lock makes two recoveries take turns over one sweep.
	place, err := storekit.LockInstallationStateTx(ctx, tx, storekit.StateCapturePartyAdmissionCursor, partyRecoveryPlace{})
	if err != nil {
		return nil, err
	}
	after, user, ceiling := place.AfterActivity, place.AfterUser, place.Ceiling
	if ceiling.IsZero() {
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT id FROM activity ORDER BY id DESC LIMIT 1),'00000000-0000-0000-0000-000000000000'::uuid)`).Scan(&ceiling); err != nil {
			return nil, err
		}
	}
	args := []any{after, user, ceiling, limit}
	query := fmt.Sprintf(`WITH sources AS (
 (SELECT DISTINCT ps.activity_id,ps.user_id FROM capture_party_source ps
 WHERE ps.admission_pending AND (ps.activity_id,ps.user_id)>($%[1]d::uuid,$%[2]d::uuid) AND ps.activity_id<=$%[3]d
 ORDER BY ps.activity_id,ps.user_id LIMIT $%[4]d)
 UNION
 (SELECT DISTINCT c.activity_id,c.user_id FROM comms_outbound c
 WHERE c.party_admission_pending AND `+sentUserMailSQL("c")+`
 AND (c.activity_id,c.user_id)>($%[1]d::uuid,$%[2]d::uuid) AND c.activity_id<=$%[3]d
 ORDER BY c.activity_id,c.user_id LIMIT $%[4]d))
 SELECT s.activity_id,s.user_id FROM sources s ORDER BY s.activity_id,s.user_id LIMIT $%[4]d`, len(args)-3, len(args)-2, len(args)-1, len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	targets, err := pgx.CollectRows(rows, pgx.RowToStructByPos[partyRecoveryTarget])
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		after, user, ceiling = ids.Nil, ids.Nil, ids.Nil
	} else {
		last := targets[len(targets)-1]
		after, user = last.Activity.UUID, last.User
	}
	// Advancing before admission bounds faults too; the next complete sweep retries them.
	return targets, storekit.StoreInstallationStateTx(ctx, tx, storekit.StateCapturePartyAdmissionCursor,
		partyRecoveryPlace{AfterActivity: after, AfterUser: user, Ceiling: ceiling})
}
