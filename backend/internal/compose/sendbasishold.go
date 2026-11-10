// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// holdSendBasisTx locks every basis row and refuses the reservation when one
// changed, stopped qualifying, or the recipient left. It also locks the
// authority the basis was read under: the seat, the team memberships it reads
// and the audience rows of every source. A change to any of them waits for the
// reservation. The clock is read after the locks, so a decision that expired
// during a wait is refused.
func holdSendBasisTx(ctx context.Context, tx pgx.Tx, basis sendBasis, clock func() time.Time) error {
	ctx, eligible, err := seatAuthorityTx(ctx, tx, basis.seat)
	if err != nil {
		return err
	}
	if !eligible {
		return errSendBasisChanged
	}
	var live int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM app_user WHERE id = $1 AND `+workforce.LiveMemberSQL("")+` FOR SHARE) seat`, basis.seat).Scan(&live); err != nil {
		return fmt.Errorf("send: holding the recipient: %w", err)
	}
	if live == 0 {
		return errSendBasisChanged
	}
	if err := holdBasisRows(ctx, tx, basis.rows); err != nil {
		return err
	}
	var sources []ids.UUID
	for _, row := range basis.rows {
		if row.table == string(recordTypeActivity) {
			sources = append(sources, row.id)
		}
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM team_membership m WHERE m.user_id = $1
		    OR (m.team_id = $2 AND m.user_id IN (SELECT assignee_id FROM activity WHERE id = ANY($3) AND archived_at IS NULL))
		   FOR SHARE OF m`, basis.seat, basis.team, basis.refs.teamTasks); err != nil {
		return fmt.Errorf("send: holding the recipient's teams: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity_audience_member am WHERE am.activity_id = ANY($1) FOR SHARE OF am`, sources); err != nil {
		return fmt.Errorf("send: holding who may read the sources: %w", err)
	}
	// The versions held, so the basis is read again under the authority this
	// transaction reads, with the clock read after the locks.
	again, err := readSendBasisWithAuthority(ctx, tx, basis.seat, basis.team, basis.refs, clock())
	if err != nil {
		return err
	}
	if !slices.Equal(again.rows, basis.rows) || len(again.excluded) != len(basis.excluded) {
		return errSendBasisChanged
	}
	return nil
}

// holdBasisRows takes a share lock on each basis row at the version read, and
// refuses when one is gone or changed. Bounded waits let a concurrent edit finish
// while yielding lock cycles to the writer before PostgreSQL breaks them.
func holdBasisRows(ctx context.Context, tx pgx.Tx, basis []basisRow) error {
	var restore string
	if err := tx.QueryRow(ctx, `WITH limits AS MATERIALIZED (
  SELECT current_setting('lock_timeout') AS previous,
   extract(epoch FROM current_setting('deadlock_timeout')::interval) * 1000 AS cycle_ms,
   extract(epoch FROM current_setting('lock_timeout')::interval) * 1000 AS current_ms)
 SELECT previous, set_config('lock_timeout',
  greatest(1, least(500, cycle_ms / 2, nullif(current_ms, 0)))::bigint::text, true)
 FROM limits`).Scan(&restore, new(string)); err != nil {
		return fmt.Errorf("send: bounding the reservation's lock wait: %w", err)
	}
	for _, table := range slices.Concat([]string{string(recordTypeActivity), "approval", "conversation_claim"}, decisionTargets) {
		var rows []ids.UUID
		var versions []int64
		for _, row := range basis {
			if row.table == table {
				rows, versions = append(rows, row.id), append(versions, row.version)
			}
		}
		if len(rows) == 0 {
			continue
		}
		var held int
		if err := tx.QueryRow(ctx, `WITH locked AS (SELECT held.id FROM `+table+` held
		       JOIN unnest($1::uuid[], $2::bigint[]) AS basis(id, version) ON basis.id = held.id AND basis.version = held.version
		       FOR SHARE OF held)
		     SELECT count(*) FROM locked`, rows, versions).Scan(&held); err != nil {
			if storekit.IsLockTimeout(err) {
				return errSendBasisChanged
			}
			return fmt.Errorf("send: holding what a message rests on: %w", err)
		}
		if held != len(rows) {
			return errSendBasisChanged
		}
	}
	for _, row := range basis {
		if err := holdBasisFingerprint(ctx, tx, row); err != nil {
			if storekit.IsLockTimeout(err) {
				return errSendBasisChanged
			}
			return err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('lock_timeout', @previous, true)`, pgx.StrictNamedArgs{"previous": restore}); err != nil {
		return fmt.Errorf("send: restoring the reservation's lock wait: %w", err)
	}
	return nil
}

func holdBasisFingerprint(ctx context.Context, tx pgx.Tx, row basisRow) error {
	if row.table == marginceChangesBasis {
		since, err := time.Parse(time.RFC3339Nano, row.key)
		if err != nil {
			return err
		}
		fingerprint, err := deals.PendingChangesFingerprintTx(ctx, tx, row.id, since, true)
		if err != nil {
			return err
		}
		if fingerprint != row.version {
			return errSendBasisChanged
		}
		return nil
	}
	if row.table != commitmentAdmissionTable {
		return nil
	}
	fingerprint, open, err := activities.HoldCommitmentAdmissionTx(ctx, tx, row.id, row.key)
	if err != nil {
		return err
	}
	if !open || fingerprint != row.version {
		return errSendBasisChanged
	}
	return nil
}
