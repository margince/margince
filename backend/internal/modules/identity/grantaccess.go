// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const grantAccessRead = string(crmcontracts.RecordGrantAccessRecordGrantAccessRead)

// GrantAccessWrite is the wider of a grant's two access levels.
const GrantAccessWrite = string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite)

// grantCut names what narrowed a write share to read, if anything did.
type grantCut string

const (
	grantCutNone grantCut = ""
	grantCutSeat grantCut = "seat"
	grantCutRole grantCut = "role"
)

// grantAccessFor is the access a share saves, and what cut it. Every grant
// writer asks it, so the cut and its audit evidence are decided here.
// A team keeps what was asked: each member's write is judged by their own role.
// A stored write grant the role cannot use is inert, because every use asks
// the role at write time. So a role change needs no rewrite of grants.
func grantAccessFor(ctx context.Context, tx pgx.Tx, in CreateGrantInput) (string, grantCut, error) {
	if in.Access != GrantAccessWrite || in.SubjectType == grantSubjectTeam {
		return in.Access, grantCutNone, nil
	}
	seat, err := seatOf(ctx, tx, in.SubjectID)
	if err != nil {
		return "", grantCutNone, err
	}
	// The seat test is auth.AdmitChangers': only a read seat is refused writes.
	// This runs at grant creation only, so whoever ships a seat-change endpoint
	// owes the other half: revoke the member's write grants there, audited.
	// Until then they are inert, as the write-authority predicate drops the
	// record_grant arm for a read seat.
	if seat == principal.SeatRead {
		if !in.FitToRecipient {
			return "", grantCutNone, fmt.Errorf("a write grant needs a full seat: %w", apperrors.ErrSeatTierInsufficient)
		}
		return grantAccessRead, grantCutSeat, nil
	}
	usable, err := roleUsesWriteShare(ctx, tx, in.RecordType, in.SubjectID)
	if err != nil {
		return "", grantCutNone, err
	}
	if !usable {
		// The answer says the role cut the share, and never names the role.
		return grantAccessRead, grantCutRole, nil
	}
	return GrantAccessWrite, grantCutNone, nil
}

// roleUsesWriteShare is whether a member's role can use a write share on a
// record type. Changing the record needs update on it. Attaching an activity
// to it needs the role's activity create.
func roleUsesWriteShare(ctx context.Context, tx pgx.Tx, recordType string, member ids.UUID) (bool, error) {
	seat := ids.From[ids.UserKind](member)
	changes, err := SeatAllows(ctx, tx, seat, recordType, principal.ActionUpdate)
	if err != nil || changes {
		return changes, err
	}
	return SeatAllows(ctx, tx, seat, "activity", principal.ActionCreate)
}

// grantCutEvidence is the audit evidence of a cut share, nil when nothing cut.
func grantCutEvidence(requested string, cut grantCut) map[string]any {
	switch cut {
	case grantCutRole:
		return map[string]any{"requested_access": requested, "limited_by_role": true}
	case grantCutSeat:
		return map[string]any{"requested_access": requested, "limited_by_seat": true}
	}
	return nil
}

func seatOf(ctx context.Context, tx pgx.Tx, member ids.UUID) (principal.SeatType, error) {
	var seat string
	if err := tx.QueryRow(ctx,
		`SELECT seat_type FROM app_user WHERE id = $1`, member).Scan(&seat); err != nil {
		return "", fmt.Errorf("read the subject's seat: %w", err)
	}
	return principal.SeatType(seat), nil
}
