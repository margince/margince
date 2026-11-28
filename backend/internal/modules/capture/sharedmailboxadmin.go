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
	// ReplyEnabled says an admin let the reader team reply as the mailbox.
	ReplyEnabled bool
	// UnreadFrom..UnreadTo is a span no sync read, which a history import
	// recovers. Both nil when there is none.
	UnreadFrom, UnreadTo *time.Time
	// HistoryNarrowing is nil unless earlier mail is still being narrowed.
	HistoryNarrowing *HistoryNarrowing
	narrowingHeld    *bool
}

const sharedMailboxColumns = `id, provider, account_label, reader_team_id, mail_posture, status, connected_by, connected_at, backup_owner_id, cardinality(coalesce(provider_scopes, '{}')) > 0, unread_from, unread_to, history_narrowing_held`

func scanSharedMailbox(row pgx.Row) (SharedMailboxView, error) {
	var v SharedMailboxView
	err := row.Scan(&v.ID, &v.Provider, &v.Address, &v.ReaderTeam, &v.Posture, &v.Status, &v.ConnectedBy, &v.ConnectedAt, &v.BackupOwner,
		&v.ReplyEnabled, &v.UnreadFrom, &v.UnreadTo, &v.narrowingHeld)
	return v, err
}

// withNarrowingTx fills in the progress of a pending history narrowing.
func (v *SharedMailboxView) withNarrowingTx(ctx context.Context, tx pgx.Tx) error {
	progress, pending, err := historyNarrowingTx(ctx, tx, mailboxScope(v.ID), v.narrowingHeld)
	if pending {
		v.HistoryNarrowing = &progress
	}
	return err
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
		  WHERE `+mailSeatSQL+` ORDER BY lower(account_label), provider`)
		if err != nil {
			return err
		}
		if out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SharedMailboxView, error) { return scanSharedMailbox(row) }); err != nil {
			return err
		}
		for i := range out {
			if err := out[i].withNarrowingTx(ctx, tx); err != nil {
				return err
			}
		}
		return nil
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
		if v, err = sharedMailboxTx(ctx, tx, id, ""); err != nil {
			return err
		}
		return v.withNarrowingTx(ctx, tx)
	})
	return v, err
}

// sharedMailboxTx reads one shared mailbox; ErrNotFound when id names none.
func sharedMailboxTx(ctx context.Context, tx pgx.Tx, id ids.UUID, lock string) (SharedMailboxView, error) {
	v, err := scanSharedMailbox(tx.QueryRow(ctx, `SELECT `+sharedMailboxColumns+` FROM capture_connection
	  WHERE id = $1 AND `+mailSeatSQL+` `+lock, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fmt.Errorf("no shared mailbox has that id: %w", apperrors.ErrNotFound)
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
	// ReplyEnabled lets the reader team reply as the mailbox, or ends that.
	ReplyEnabled *bool
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
		return "name the reader team, the mail visibility, the backup owner or the replies to change"
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
	if in.ReaderTeam == nil && in.Posture == nil && in.BackupOwner == nil && !in.ClearBackupOwner && in.ReplyEnabled == nil {
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
		out, err = changeSharedMailboxTx(ctx, tx, id, in, r.sharedSendScope)
		return err
	})
	return out, err
}

func changeSharedMailboxTx(ctx context.Context, tx pgx.Tx, id ids.UUID, in SharedChange, sendScope func(provider string) (string, error)) (SharedMailboxView, error) {
	prior, err := sharedMailboxTx(ctx, tx, id, "FOR UPDATE")
	if err != nil {
		return prior, err
	}
	next := prior
	if in.ReaderTeam != nil && *in.ReaderTeam != prior.ReaderTeam {
		if next, err = moveSharedReadersTx(ctx, tx, prior, *in.ReaderTeam); err != nil {
			return prior, err
		}
	}
	if in.Posture != nil && *in.Posture != prior.Posture {
		if next, err = changeSharedPostureTx(ctx, tx, next, *in.Posture); err != nil {
			return prior, err
		}
	}
	if next, err = changeBackupOwnerTx(ctx, tx, next, in); err != nil {
		return prior, err
	}
	if in.ReplyEnabled != nil && *in.ReplyEnabled != next.ReplyEnabled {
		if next, err = changeSharedReplyTx(ctx, tx, next, *in.ReplyEnabled, sendScope); err != nil {
			return prior, err
		}
	}
	if next, err = narrowSharedHistoryTx(ctx, tx, next, in); err != nil {
		return prior, err
	}
	if err := next.withNarrowingTx(ctx, tx); err != nil {
		return prior, err
	}
	return next, nil
}

// narrowSharedHistoryTx asks for the earlier mail to be narrowed when the change
// says so, whether or not the visibility moved. An admin may narrow the history
// of a mailbox that is already held. It answers the mailbox as stored after.
func narrowSharedHistoryTx(ctx context.Context, tx pgx.Tx, next SharedMailboxView, in SharedChange) (SharedMailboxView, error) {
	if !in.ApplyToHistory || in.Posture == nil {
		return next, nil
	}
	if err := requestHistoryNarrowingTx(ctx, tx, sharedConnection(next.ID), next.Posture); err != nil {
		return next, err
	}
	return sharedMailboxTx(ctx, tx, next.ID, "")
}

// moveSharedReadersTx hands the mailbox to another reader team, and drops a
// backup owner who is not in it.
func moveSharedReadersTx(ctx context.Context, tx pgx.Tx, prior SharedMailboxView, team ids.UUID) (SharedMailboxView, error) {
	if err := refuseUnreadableTeamTx(ctx, tx, team); err != nil {
		return prior, err
	}
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET reader_team_id = $2
	  WHERE id = $1 AND reader_team_id = $3`, prior.ID, team, prior.ReaderTeam)
	if err != nil {
		return prior, fmt.Errorf("capture: moving the shared mailbox to other readers: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return prior, fmt.Errorf("the shared mailbox's readers changed under this move: %w", apperrors.ErrConflict)
	}
	next := prior
	next.ReaderTeam = team
	if err := announceSharedChangeTx(ctx, tx, sharedChangeReaders, prior, next); err != nil {
		return prior, err
	}
	return dropBackupOutsideTeamTx(ctx, tx, next)
}

// changeSharedPostureTx sets who may read the mailbox's mail from now on.
func changeSharedPostureTx(ctx context.Context, tx pgx.Tx, before SharedMailboxView, posture string) (SharedMailboxView, error) {
	if err := refuseSharedWithoutOptInTx(ctx, tx, posture); err != nil {
		return before, err
	}
	if _, err := replacePostureTx(ctx, tx, sharedConnection(before.ID), posture); err != nil {
		return before, fmt.Errorf("capture: setting the shared mailbox's visibility: %w", err)
	}
	next := before
	next.Posture = posture
	return next, announceSharedChangeTx(ctx, tx, sharedChangeVisibility, before, next)
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
		  WHERE id = $1 AND `+mailSeatSQL, id).Scan(&live)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live) {
			return fmt.Errorf("no connected shared mailbox has that id: %w", apperrors.ErrNotFound)
		}
		if err != nil {
			return err
		}
		run, err = r.openConnectionBackfillTx(ctx, tx, id, ids.UserID{}, windowMonths, estimate, enqueue, backfillStart{resume: true, shared: true})
		return err
	})
	return run, err
}
