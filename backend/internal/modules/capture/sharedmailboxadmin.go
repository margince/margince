// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SharedMailboxView is one shared mailbox as its admin screen shows it.
type SharedMailboxView struct {
	ID          ids.UUID
	Provider    string
	Address     string
	ReaderTeam  ids.UUID
	Posture     string
	Status      string
	ConnectedBy *ids.UUID
	ConnectedAt time.Time
	// BackupOwner holds the work of mail whose sender no reader owns.
	BackupOwner *ids.UUID
}

const sharedMailboxColumns = `id, provider, account_label, reader_team_id, mail_posture, status, connected_by, connected_at, backup_owner_id`

func scanSharedMailbox(row pgx.Row) (SharedMailboxView, error) {
	var v SharedMailboxView
	err := row.Scan(&v.ID, &v.Provider, &v.Address, &v.ReaderTeam, &v.Posture, &v.Status, &v.ConnectedBy, &v.ConnectedAt, &v.BackupOwner)
	return v, err
}

// SharedMailboxes lists the installation's shared mailboxes, disconnected ones
// included: their history stays with their readers.
func (r *Registry) SharedMailboxes(ctx context.Context) ([]SharedMailboxView, error) {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return nil, err
	}
	var out []SharedMailboxView
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+sharedMailboxColumns+` FROM capture_connection
		  WHERE user_id IS NULL AND archived_at IS NULL ORDER BY lower(account_label), provider`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SharedMailboxView, error) { return scanSharedMailbox(row) })
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing the shared mailboxes: %w", err)
	}
	return out, nil
}

// SharedMailbox reads one shared mailbox for its admin.
func (r *Registry) SharedMailbox(ctx context.Context, id ids.UUID) (SharedMailboxView, error) {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return SharedMailboxView{}, err
	}
	var v SharedMailboxView
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		v, err = sharedMailboxTx(ctx, tx, id, "")
		return err
	})
	return v, err
}

// sharedMailboxTx reads one shared mailbox; ErrNotFound when id names none.
func sharedMailboxTx(ctx context.Context, tx pgx.Tx, id ids.UUID, lock string) (SharedMailboxView, error) {
	v, err := scanSharedMailbox(tx.QueryRow(ctx, `SELECT `+sharedMailboxColumns+` FROM capture_connection
	  WHERE id = $1 AND user_id IS NULL AND archived_at IS NULL `+lock, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fmt.Errorf("capture: no shared mailbox %s: %w", id, apperrors.ErrNotFound)
	}
	return v, err
}

// SharedChange moves a shared mailbox to other readers or another visibility,
// or names its backup owner.
type SharedChange struct {
	ReaderTeam *ids.UUID
	Posture    *string
	// BackupOwner names the backup owner; ClearBackupOwner removes it.
	BackupOwner      *ids.UUID
	ClearBackupOwner bool
	// ApplyToHistory narrows mail already captured to a stricter visibility.
	ApplyToHistory bool
	// Confirmed is the admin's yes: a change moves the whole captured history.
	Confirmed bool
}

// SharedChangeError refuses a change the admin did not confirm, or one that
// changes nothing.
type SharedChangeError struct{ empty bool }

func (e *SharedChangeError) Error() string {
	if e.empty {
		return "name the reader team, the mail visibility or the backup owner to change"
	}
	return "this moves every message the mailbox captured to other readers; send confirm: true to go ahead"
}

// FieldFault answers the refusal on the field the caller must change.
func (e *SharedChangeError) FieldFault() (field, code, message string) {
	if e.empty {
		return fieldReaderTeam, "nothing_to_change", e.Error()
	}
	return "confirm", "confirm_required", e.Error()
}

// ChangeSharedMailbox changes who reads a shared mailbox, or its visibility.
// The row, its audit row and its event commit together.
func (r *Registry) ChangeSharedMailbox(ctx context.Context, id ids.UUID, in SharedChange) (SharedMailboxView, error) {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return SharedMailboxView{}, err
	}
	if in.ReaderTeam == nil && in.Posture == nil && in.BackupOwner == nil && !in.ClearBackupOwner {
		return SharedMailboxView{}, &SharedChangeError{empty: true}
	}
	// Only a move of the readers or the visibility moves captured history.
	if (in.ReaderTeam != nil || in.Posture != nil) && !in.Confirmed {
		return SharedMailboxView{}, &SharedChangeError{}
	}
	if in.Posture != nil {
		if err := refuseSharedVisibility(*in.Posture); err != nil {
			return SharedMailboxView{}, err
		}
	}
	var out SharedMailboxView
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = r.changeSharedMailboxTx(ctx, tx, id, in)
		return err
	})
	return out, err
}

func (r *Registry) changeSharedMailboxTx(ctx context.Context, tx pgx.Tx, id ids.UUID, in SharedChange) (SharedMailboxView, error) {
	prior, err := sharedMailboxTx(ctx, tx, id, "FOR UPDATE")
	if err != nil {
		return prior, err
	}
	next := prior
	if in.ReaderTeam != nil && *in.ReaderTeam != prior.ReaderTeam {
		if err := refuseUnreadableTeamTx(ctx, tx, *in.ReaderTeam); err != nil {
			return prior, err
		}
		tag, err := tx.Exec(ctx, `UPDATE capture_connection SET reader_team_id = $2
		  WHERE id = $1 AND reader_team_id = $3 AND archived_at IS NULL`, id, *in.ReaderTeam, prior.ReaderTeam)
		if err != nil {
			return prior, fmt.Errorf("capture: moving the shared mailbox to other readers: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return prior, fmt.Errorf("capture: the shared mailbox's readers changed under this move: %w", apperrors.ErrConflict)
		}
		next.ReaderTeam = *in.ReaderTeam
		if err := announceSharedChangeTx(ctx, tx, sharedChangeReaders, prior, next); err != nil {
			return prior, err
		}
		if next, err = dropBackupOutsideTeamTx(ctx, tx, next); err != nil {
			return prior, err
		}
	}
	if in.Posture != nil && *in.Posture != prior.Posture {
		if err := refuseSharedWithoutOptInTx(ctx, tx, *in.Posture); err != nil {
			return prior, err
		}
		if _, err := replacePostureTx(ctx, tx, sharedConnection(id), *in.Posture); err != nil {
			return prior, fmt.Errorf("capture: setting the shared mailbox's visibility: %w", err)
		}
		before := next
		next.Posture = *in.Posture
		if err := announceSharedChangeTx(ctx, tx, sharedChangeVisibility, before, next); err != nil {
			return prior, err
		}
	}
	if next, err = changeBackupOwnerTx(ctx, tx, next, in); err != nil {
		return prior, err
	}
	// Asked whether or not the visibility moved: an admin may narrow the
	// history of a mailbox that is already held.
	if in.ApplyToHistory && in.Posture != nil {
		return next, r.narrowSharedHistoryTx(ctx, tx, next)
	}
	return next, nil
}

// narrowSharedHistoryTx applies the mailbox's visibility to the mail it already
// captured, and records that it did on the mailbox's trail.
func (r *Registry) narrowSharedHistoryTx(ctx context.Context, tx pgx.Tx, box SharedMailboxView) error {
	if err := r.narrowHistoryTo(ctx, tx, mailboxImporter(box.ID), box.Posture); err != nil {
		return err
	}
	image := sharedMailboxImage(box)
	narrowed := sharedMailboxImage(box)
	narrowed["history_narrowed_to"] = box.Posture
	return announceSharedMailboxTx(ctx, tx, auditVerbUpdate, sharedChangeVisibility, box.ID, image, narrowed)
}

func announceSharedChangeTx(ctx context.Context, tx pgx.Tx, change string, before, after SharedMailboxView) error {
	return announceSharedMailboxTx(ctx, tx, auditVerbUpdate, change, after.ID,
		sharedMailboxImage(before), sharedMailboxImage(after))
}

// DisconnectShared stops a shared mailbox's capture and destroys its
// credential. The row, its mail and its readers stay, so a reconnect of the
// same address picks the row up again.
func (r *Registry) DisconnectShared(ctx context.Context, id ids.UUID) error {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return err
	}
	var box SharedMailboxView
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		box, err = sharedMailboxTx(ctx, tx, id, "")
		return err
	}); err != nil {
		return err
	}
	return r.withdrawAndDestroy(ctx, box.Provider, sharedConnection(id))
}

// StartSharedBackfill starts a shared mailbox's history import. It runs as the
// mailbox's own sync does, with no seat behind it.
func (r *Registry) StartSharedBackfill(ctx context.Context, id ids.UUID, windowMonths int, estimate connector.BackfillEstimate, enqueue EnqueueBackfill) (BackfillRun, error) {
	if _, err := RequireSharedMailboxAdmin(ctx); err != nil {
		return BackfillRun{}, err
	}
	if err := r.admitsBackfillStart(windowMonths, enqueue); err != nil {
		return BackfillRun{}, err
	}
	var run BackfillRun
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var live bool
		err := tx.QueryRow(ctx, `SELECT status IN ('connected','error') FROM capture_connection
		  WHERE id = $1 AND user_id IS NULL AND archived_at IS NULL`, id).Scan(&live)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live) {
			return fmt.Errorf("capture: no connected shared mailbox %s: %w", id, apperrors.ErrNotFound)
		}
		if err != nil {
			return err
		}
		run, err = r.openConnectionBackfillTx(ctx, tx, id, ids.UserID{}, windowMonths, estimate, enqueue, backfillStart{resume: true, shared: true})
		return err
	})
	return run, err
}
