// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package scopesubjects reads the named subjects an analytics scope measures.
// Each read is also the check that the subject is still live. Inside one
// reporting evaluation the snapshot answers each subject once (snapshotreads).
package scopesubjects

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/snapshotreads"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// key tells the three reads apart in a snapshot's memo.
type key struct {
	kind  string
	id    ids.UUID
	teams string
}

// SharesATeam asks whether one user is a live member of any of these teams.
//
// The membership read is what makes a team manager's owner scope safe: without
// it a manager could name any user id in the installation and measure them.
func SharesATeam(ctx context.Context, tx pgx.Tx, teams []ids.UUID, user ids.UUID) (bool, error) {
	if len(teams) == 0 {
		return false, nil
	}
	return snapshotreads.Read(ctx, tx, key{"membership", user, snapshotreads.IDSet(teams)}, func() (bool, error) {
		var shares bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM team_membership tm
			JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
			JOIN app_user u ON u.id = tm.user_id AND `+identity.LiveMemberSQL("u")+`
			WHERE tm.user_id = $1 AND tm.team_id = ANY($2))`, user, teams).Scan(&shares)
		if err != nil {
			return false, fmt.Errorf("scopesubjects: reading team membership: %w", err)
		}
		return shares, nil
	})
}

// OwnerLabel and TeamLabel name a population for the reader.
//
// Both refuse an archived subject as absent rather than labelling it. A scope
// resolved against a team that was archived after the request was written is
// not a population any more. Answering over it would report a set the
// installation no longer has.

func OwnerLabel(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	return snapshotreads.Read(ctx, tx, key{kind: "owner", id: id}, func() (string, error) {
		var label string
		err := tx.QueryRow(ctx,
			`SELECT display_name FROM app_user WHERE id = $1 AND `+identity.LiveMemberSQL(""), id).Scan(&label)
		if errors.Is(err, pgx.ErrNoRows) {
			// Deactivated counts as absent, not merely archived: a departed
			// colleague is no longer a population somebody measures.
			return "", fmt.Errorf("no such contact to measure: %w", apperrors.ErrNotFound)
		}
		if err != nil {
			return "", fmt.Errorf("scopesubjects: naming the measured contact: %w", err)
		}
		return label, nil
	})
}

func TeamLabel(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	return snapshotreads.Read(ctx, tx, key{kind: "team", id: id}, func() (string, error) {
		var label string
		err := tx.QueryRow(ctx,
			`SELECT name FROM team WHERE id = $1 AND archived_at IS NULL`, id).Scan(&label)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no such team to measure: %w", apperrors.ErrNotFound)
		}
		if err != nil {
			return "", fmt.Errorf("scopesubjects: naming the measured team: %w", err)
		}
		return label, nil
	})
}
