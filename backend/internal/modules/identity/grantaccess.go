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

// grantAccessWrite is the wider of a grant's two access levels.
const grantAccessWrite = string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite)

// writeReach is whether a member could use a write grant on one record type.
// The seat ceiling (AAD-AC-4) and the role's update permission both refuse
// every write the grant would seem to allow.
type writeReach struct {
	seat bool
	role bool
}

func memberWriteReach(ctx context.Context, tx pgx.Tx, recordType string, member ids.UUID) (writeReach, error) {
	seat, err := seatOf(ctx, tx, member)
	if err != nil {
		return writeReach{}, err
	}
	grants, err := loadGrantsFor(ctx, tx, []ids.UUID{member})
	if err != nil {
		return writeReach{}, fmt.Errorf("read the subject's role: %w", err)
	}
	return writeReach{
		seat: seat.CanMutate(),
		role: grants[member].perms.Allows(recordType, principal.ActionUpdate),
	}, nil
}

// grantAccessFor is the access an explicit share saves, and whether the
// subject's role cut write to read. A read seat is refused, as the contract
// states; the answer names no role.
//
// A team keeps what was asked: its members hold different roles, and each
// write is judged by the writer's own.
func grantAccessFor(ctx context.Context, tx pgx.Tx, in CreateGrantInput) (access string, limitedByRole bool, err error) {
	if in.Access != grantAccessWrite || in.SubjectType == grantSubjectTeam {
		return in.Access, false, nil
	}
	reach, err := memberWriteReach(ctx, tx, in.RecordType, in.SubjectID)
	if err != nil {
		return "", false, err
	}
	if !reach.seat {
		return "", false, fmt.Errorf("a write grant needs a full seat: %w", apperrors.ErrSeatTierInsufficient)
	}
	if !reach.role {
		return grantAccessRead, true, nil
	}
	return grantAccessWrite, false, nil
}

// ShareAccessForTx is the widest access a share may give this member on one
// record type: write when both seat and role allow it, read otherwise.
func ShareAccessForTx(ctx context.Context, tx pgx.Tx, recordType string, member ids.UUID) (string, error) {
	reach, err := memberWriteReach(ctx, tx, recordType, member)
	if err != nil {
		return "", err
	}
	if reach.seat && reach.role {
		return grantAccessWrite, nil
	}
	return grantAccessRead, nil
}

func seatOf(ctx context.Context, tx pgx.Tx, member ids.UUID) (principal.SeatType, error) {
	var seat string
	if err := tx.QueryRow(ctx,
		`SELECT seat_type FROM app_user WHERE id = $1`, member).Scan(&seat); err != nil {
		return "", fmt.Errorf("read the subject's seat: %w", err)
	}
	return principal.SeatType(seat), nil
}
