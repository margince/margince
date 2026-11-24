// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type backupActingKey struct{}

// HeldAuthority reads a member's authority inside tx and holds it to commit
// (identity.HeldAuthorityTx).
type HeldAuthority func(context.Context, pgx.Tx, ids.UUID) (authz.RBAC, principal.SeatType, error)

// backupOwnerActingTx binds the backup owner's authority as of tx, so mail from
// an unknown sender may become their contact. A mailbox with no backup owner,
// or one who left the reader team or may no longer capture, binds nothing and
// stays link-only. Call it before tx locks a row.
func (s *Sink) backupOwnerActingTx(ctx context.Context, tx pgx.Tx, box ids.UUID) (context.Context, error) {
	if s.partyAuthority == nil {
		return ctx, nil
	}
	var team ids.UUID
	var backup *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT reader_team_id, backup_owner_id FROM capture_connection WHERE id = $1`,
		box).Scan(&team, &backup); err != nil {
		return ctx, fmt.Errorf("capture: reading the shared mailbox's backup owner: %w", err)
	}
	if backup == nil {
		return ctx, nil
	}
	acting, holds, err := backupMayHoldTx(ctx, tx, s.partyAuthority.HeldAuthorityTx, team, *backup)
	if err != nil || !holds {
		return ctx, err
	}
	run, _ := principal.Actor(ctx)
	acting.ID = run.ID
	return context.WithValue(ctx, backupActingKey{}, acting), nil
}

// backupMayHoldTx answers whether backup may still hold a shared mailbox's
// work: a live seat in the reader team whose authority may capture. It takes
// the authorization lock first, so a team or role change waits for this write.
func backupMayHoldTx(ctx context.Context, tx pgx.Tx, authority HeldAuthority, team, backup ids.UUID) (principal.Principal, bool, error) {
	rbac, seat, err := authority(ctx, tx, backup)
	if errors.Is(err, apperrors.ErrNotFound) {
		return principal.Principal{}, false, nil
	}
	if err != nil {
		return principal.Principal{}, false, err
	}
	err = refuseBackupOutsideTeamTx(ctx, tx, team, &backup)
	if _, outside := errors.AsType[*BackupOwnerError](err); outside {
		return principal.Principal{}, false, nil
	}
	if err != nil {
		return principal.Principal{}, false, err
	}
	if mayCapture := CaptureRefusal(rbac.Permissions, seat) == nil; !mayCapture {
		return principal.Principal{}, false, nil
	}
	acting := authz.HumanPrincipal(backup, rbac, seat)
	acting.Type, acting.OnBehalfOf = principal.PrincipalConnector, backup
	return acting, true, nil
}

// backupKeepsOutTx reports whether the backup owner's own rules keep this mail
// from them: an exclusion they set, or a thread they judged private.
func backupKeepsOutTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, activity, backup ids.UUID) (bool, error) {
	excluded, err := excludedForTx(ctx, tx, rec, backup)
	if err != nil || excluded != "" {
		return excluded != "", err
	}
	return storedMessageIsPrivateTx(ctx, tx, ids.From[ids.ActivityKind](activity), backup)
}

// backupActingOf answers the backup owner a shared mailbox run may mint for.
func backupActingOf(ctx context.Context) (principal.Principal, bool) {
	acting, ok := ctx.Value(backupActingKey{}).(principal.Principal)
	return acting, ok
}

// BackupOwnerError refuses a backup owner who could not hold the mailbox's
// work. They must be an active seat that can hold work, in its reader team.
type BackupOwnerError struct{}

func (e *BackupOwnerError) Error() string {
	return "the backup owner must be an active member of the reader team who can hold work"
}

// FieldFault answers the refusal on the backup owner field.
func (e *BackupOwnerError) FieldFault() (field, code, message string) {
	return fieldBackupOwner, "backup_owner_not_in_reader_team", e.Error()
}

// refuseBackupOutsideTeamTx admits a backup owner who is a live member of the
// reader team and may hold work. The work rule asks the same again each time
// it runs, so a seat that later leaves the team gets no more work.
func refuseBackupOutsideTeamTx(ctx context.Context, tx pgx.Tx, team ids.UUID, backup *ids.UUID) error {
	if backup == nil {
		return nil
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_membership m
	  JOIN app_user u ON u.id = m.user_id AND `+auth.AssigneeEligibleSQL("u")+`
	 WHERE m.team_id = $1 AND m.user_id = $2)`, team, *backup).Scan(&member); err != nil {
		return fmt.Errorf("capture: reading whether the backup owner is in the reader team: %w", err)
	}
	if !member {
		return &BackupOwnerError{}
	}
	return nil
}

// changeBackupOwnerTx names or clears the backup owner of the locked
// mailbox next, with its audit row and event, and answers the mailbox after.
func changeBackupOwnerTx(ctx context.Context, tx pgx.Tx, next SharedMailboxView, in SharedChange) (SharedMailboxView, error) {
	want := in.BackupOwner
	if in.ClearBackupOwner {
		want = nil
	} else if want == nil {
		return next, nil
	}
	if err := refuseBackupOutsideTeamTx(ctx, tx, next.ReaderTeam, want); err != nil {
		return next, err
	}
	if sameSeat(want, next.BackupOwner) {
		return next, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET backup_owner_id = $2
	  WHERE id = $1 AND user_id IS NULL AND archived_at IS NULL`, next.ID, want)
	if err != nil {
		return next, fmt.Errorf("capture: naming the shared mailbox's backup owner: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return next, fmt.Errorf("the shared mailbox changed under this edit: %w", apperrors.ErrConflict)
	}
	before := next
	next.BackupOwner = want
	return next, announceSharedChangeTx(ctx, tx, sharedChangeBackupOwner, before, next)
}

func sameSeat(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// dropBackupOutsideTeamTx clears the backup owner of mailbox next when its new
// reader team does not hold them. A mailbox never names a backup owner outside
// its readers.
func dropBackupOutsideTeamTx(ctx context.Context, tx pgx.Tx, next SharedMailboxView) (SharedMailboxView, error) {
	err := refuseBackupOutsideTeamTx(ctx, tx, next.ReaderTeam, next.BackupOwner)
	if _, outside := errors.AsType[*BackupOwnerError](err); !outside {
		return next, err
	}
	return changeBackupOwnerTx(ctx, tx, next, SharedChange{ClearBackupOwner: true})
}
