// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// The changes a shared mailbox announces.
const (
	sharedChangeConnected     = "connected"
	sharedChangeReconnected   = "reconnected"
	sharedChangeReaders       = "readers_changed"
	sharedChangeVisibility    = "visibility_changed"
	sharedChangeBackupOwner   = "backup_owner_changed"
	sharedChangeDisconnected  = "disconnected"
	sharedChangeHistoryImport = "history_import_started"
)

// fieldReaderTeam names the reader team on the wire and on the audit trail.
const fieldReaderTeam = "reader_team_id"

// fieldBackupOwner names the backup owner on the wire and on the audit trail.
const fieldBackupOwner = "backup_owner_id"

// The audit verbs a shared mailbox's changes take.
const (
	auditVerbCreate = "create"
	auditVerbUpdate = "update"
)

// announceSharedMailboxTx writes the audit row and the outbox event of one
// change to a shared mailbox, in the change's own transaction.
func announceSharedMailboxTx(ctx context.Context, tx pgx.Tx, verb, change string, id ids.UUID, before, after map[string]any) error {
	auditID, err := storekit.Audit(ctx, tx, verb, captureConnectionObject, id, before, after)
	if err != nil {
		return fmt.Errorf("capture: auditing the %s of shared mailbox %s: %w", verb, id, err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.InternalEventSharedMailboxChanged{
		Change: crmcontracts.InternalEventSharedMailboxChangedChange(change),
	})
}

// sharedMailboxImage is one side of a shared mailbox's audit trail. It names
// the readers, the backup owner and the visibility, never the credential.
func sharedMailboxImage(v SharedMailboxView) map[string]any {
	return map[string]any{
		auditKeyProvider: v.Provider, "account_label": v.Address, columnStatus: v.Status,
		fieldReaderTeam: v.ReaderTeam, auditKeyPosture: v.Posture, fieldBackupOwner: v.BackupOwner,
	}
}

// RequireSharedMailboxAdmin admits a signed-in admin, never an agent or the system. A
// shared mailbox decides who reads a whole team's mail.
func RequireSharedMailboxAdmin(ctx context.Context) (principal.Principal, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return principal.Principal{}, fmt.Errorf("capture: only a signed-in admin manages a shared mailbox: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.RequireAdmin(ctx); err != nil {
		return principal.Principal{}, fmt.Errorf("capture: only an admin manages a shared mailbox: %w", err)
	}
	return actor, nil
}

// ReaderTeamError refuses a reader team that cannot read: unknown, archived,
// or with no live member.
type ReaderTeamError struct{ empty bool }

func (e *ReaderTeamError) Error() string {
	if e.empty {
		return "the reader team has no active member, so nobody could read this mailbox"
	}
	return "the reader team does not exist or is archived"
}

// FieldFault answers the refusal on the field that named the team.
func (e *ReaderTeamError) FieldFault() (field, code, message string) {
	if e.empty {
		return fieldReaderTeam, "reader_team_has_no_member", e.Error()
	}
	return fieldReaderTeam, "reader_team_not_found", e.Error()
}

// refuseUnreadableTeamTx holds the reader team for share until commit. An
// archive of the team then waits, and afterwards sees the mailbox.
func refuseUnreadableTeamTx(ctx context.Context, tx pgx.Tx, team ids.UUID) error {
	var held ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM team WHERE id = $1 AND archived_at IS NULL FOR SHARE`, team).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ReaderTeamError{}
	}
	if err != nil {
		return fmt.Errorf("capture: reading the reader team: %w", err)
	}
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_membership m
	  JOIN app_user u ON u.id = m.user_id AND `+workforce.LiveMemberSQL("u")+`
	 WHERE m.team_id = $1)`, team).Scan(&live); err != nil {
		return fmt.Errorf("capture: reading the reader team's members: %w", err)
	}
	if !live {
		return &ReaderTeamError{empty: true}
	}
	return nil
}

// SharedVisibilityError refuses a visibility a shared mailbox cannot have.
// The classifier is never asked about a shared mailbox.
type SharedVisibilityError struct{ Visibility string }

func (e *SharedVisibilityError) Error() string {
	return "a shared mailbox is held or shared, never " + e.Visibility
}

// FieldFault answers the refusal on the visibility field.
func (e *SharedVisibilityError) FieldFault() (field, code, message string) {
	return "mail_visibility", "invalid_mail_visibility", e.Error()
}

func refuseSharedVisibility(posture string) error {
	if posture == PostureHeld || posture == PostureShared {
		return nil
	}
	return &SharedVisibilityError{Visibility: posture}
}
