// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A shared mailbox (sales@, an operations desk) is a capture_connection no
// seat owns. Its user_id is NULL, an admin connected it, and one team reads
// what it brings in. Its sync is the personal sync with no member behind it, so
// no seat's authority, verdicts or holds apply.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sharedMailboxLiveSQL is a shared row, aliased `c`, that a sync may still run
// for: its reader team exists and is not archived. A mailbox whose readers are
// gone captures mail nobody may read.
const sharedMailboxLiveSQL = `(c.user_id IS NULL AND EXISTS (SELECT 1 FROM team reader_team
	  WHERE reader_team.id = c.reader_team_id AND reader_team.archived_at IS NULL))`

type sharedMailboxKey struct{}

// withSharedMailbox marks a run as the sync of the shared mailbox connection.
func withSharedMailbox(ctx context.Context, connection ids.UUID) context.Context {
	return context.WithValue(ctx, sharedMailboxKey{}, connection)
}

// sharedMailboxOf answers the shared connection a run syncs, when it syncs one.
func sharedMailboxOf(ctx context.Context) (ids.UUID, bool) {
	connection, ok := ctx.Value(sharedMailboxKey{}).(ids.UUID)
	return connection, ok && !connection.IsZero()
}

// sharedMailboxPrincipal acts for the connector alone. A capture that mints no
// contact, lead or task writes only the activity, and reads the records an
// address or a link files it under. captured_by is then the bare connector id,
// which auth.ConnectorSeatSQL reads as no owner.
func sharedMailboxPrincipal(name string) principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   connectorPrincipalID(name),
		Permissions: principal.Permissions{
			RoleKeys: []string{"shared_mailbox"},
			Objects: map[string]principal.ObjectGrant{
				capturedActivityObject: {Create: true, Read: true},
				capturedContactObject:  {Read: true},
				capturedLeadObject:     {Read: true},
				"company":              {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	}
}

// runContext is the context one sync, history page or watch renewal runs
// under. A personal row runs with the granting member's live authority. A
// shared one (grantedBy nil) runs as the connector alone.
func (r *Registry) runContext(ctx context.Context, name string, grantedBy *ids.UserID, connection ids.UUID) (context.Context, error) {
	if grantedBy != nil {
		return r.connectorContext(ctx, name, *grantedBy)
	}
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return nil, errors.New("capture: sync outside workspace context")
	}
	shared := principal.WithActor(withSharedMailbox(ctx, connection), sharedMailboxPrincipal(name))
	return asMailboxRun(principal.WithCorrelationID(shared, ids.NewV7())), nil
}

// sharedMailboxAllowedTx admits a write of a shared mailbox run while its row
// is still a live shared mailbox. The run carries no member to ask
// CaptureRefusal of, so this check takes its place.
func sharedMailboxAllowedTx(ctx context.Context, tx pgx.Tx, connection ids.UUID) error {
	var live bool
	err := tx.QueryRow(ctx, `SELECT `+sharedMailboxLiveSQL+`
	  FROM capture_connection c WHERE c.id = $1 AND c.archived_at IS NULL AND c.status IN ('connected','error')`,
		connection).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live) {
		return fmt.Errorf("capture: the shared mailbox is no longer connected to a live reader team: %w", apperrors.ErrPermissionDenied)
	}
	return err
}

// sharedMailboxAddressTx is the shared mailbox's own address, the one identity
// its run holds: proof of delivery, and never a counterparty.
func sharedMailboxAddressTx(ctx context.Context, tx pgx.Tx, connection ids.UUID) (SelfSet, error) {
	return database.Remember(tx, ownerIdentitiesMemo+"mailbox:"+connection.String(), func() (SelfSet, error) {
		var label string
		if err := tx.QueryRow(ctx, `SELECT coalesce(account_label, '') FROM capture_connection
		  WHERE id = $1 AND user_id IS NULL AND archived_at IS NULL`, connection).Scan(&label); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return SelfSet{}, nil
			}
			return SelfSet{}, fmt.Errorf("capture: reading the shared mailbox's address: %w", err)
		}
		return NewSelfSet([]string{bareAddress(label)}, nil), nil
	})
}

// postureReason is the word a hold by the delivering mailbox's own posture
// records.
func postureReason(ctx context.Context) string {
	if _, shared := sharedMailboxOf(ctx); shared {
		return audienceReasonSharedMailbox
	}
	return audienceReasonPosture
}

// SharedMailboxesReadByTx names the shared mailboxes a team reads, by their
// address, the disconnected ones included: their history stays readable. Identity asks it before a team change widens or ends a read.
func SharedMailboxesReadByTx(ctx context.Context, tx pgx.Tx, team ids.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT c.account_label FROM capture_connection c
	  WHERE `+auth.SharedMailboxOfTeamSQL("c", "$1")+` ORDER BY lower(c.account_label)`, team)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the shared mailboxes a team reads: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
