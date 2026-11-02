// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// PartySourceOriginal retains the original owner proof during historical source repair.
type PartySourceOriginal struct {
	Activity ids.ActivityID
	Original ids.UUID
	User     ids.UUID
	Provider string
	Record   connector.NormalizedRecord
}

// RestorePartySourceTx recovers recipient proof without inventing a historical folder.
func RestorePartySourceTx(ctx context.Context, tx pgx.Tx, in PartySourceOriginal) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	var attested bool
	err := tx.QueryRow(ctx, `SELECT a.counterparty_outbound_attested FROM activity a
 WHERE a.id=@activity AND split_part(a.captured_by,':',2)=@provider
 AND `+auth.ConnectorSeatSQL("a")+`=@owner FOR SHARE`, pgx.NamedArgs{partyActivityArg: in.Activity, partyProviderArg: in.Provider, "owner": in.User.String()}).Scan(&attested)
	if err != nil {
		return err
	}
	fields, ok := in.Record.Fields.(ActivityFields)
	if !ok {
		return apperrors.ErrInvalidArgument
	}
	evidence := partyEvidenceFor(in.Activity, in.User, in.Provider, in.Record, fields)
	evidence.Sent = attested && evidence.Sent
	evidence.ContainersKnown = false
	evidence.Original = &in.Original
	return recordPartySourceTx(ctx, tx, evidence)
}

// MarkPartySourceReplayedTx keeps one unreadable original from starving later replay work.
func MarkPartySourceReplayedTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, outcome string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return markPartySourceKnownTx(ctx, tx, activity, outcome)
}

func markPartySourceKnownTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, outcome string) error {
	_, err := tx.Exec(ctx, `INSERT INTO capture_party_source_replay(activity_id,outcome) VALUES(@activity,@outcome) ON CONFLICT(activity_id) DO NOTHING`, pgx.NamedArgs{partyActivityArg: activity, "outcome": outcome})
	return err
}
