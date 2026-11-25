// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/extingress"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// IngestBatch lands the records one at a time through Ingest, then saves the
// position. The save is a separate commit after the last record is answered.
// A stop in the middle leaves the old position, so the next run offers the
// batch again. The natural key makes that replay a no-op.
//
// A refusal ends the batch where it happened. The results answered so far come
// back with the error and the position stays where it was.
func (r *callRuntime) IngestBatch(ctx context.Context, on extension.UserID, system extension.SourceName, recs []extension.Record, position extension.BatchPosition) ([]extension.Result, error) {
	results, err := r.landBatch(ctx, on, system, recs, position, false)
	if err != nil {
		return results, err
	}
	return results, r.inPositionTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return extingress.SavePosition(ctx, tx, r.unit, string(system), position)
	})
}

func (r *callRuntime) IngestPosition(ctx context.Context, system extension.SourceName) (extension.BatchPosition, error) {
	if err := r.refusePositionKind(system, false); err != nil {
		return "", err
	}
	var saved extension.MemberPosition
	err := r.inPositionTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		saved, err = extingress.ReadPosition(ctx, tx, r.unit, string(system), extingress.NoMember)
		return err
	})
	return saved.Position, err
}

func (r *callRuntime) IngestMemberBatch(ctx context.Context, on extension.UserID, system extension.SourceName, recs []extension.Record, position extension.BatchPosition, expected extension.PositionVersion) (extension.MemberBatchResult, error) {
	member, err := r.consentedMember(ctx, on)
	if err != nil {
		return extension.MemberBatchResult{}, err
	}
	results, err := r.landBatch(ctx, on, system, recs, position, true)
	out := extension.MemberBatchResult{Results: results}
	if err != nil {
		return out, err
	}
	err = r.inPositionTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		out.Position, err = extingress.SaveMemberPosition(ctx, tx, r.unit, string(system), member, position, expected)
		return err
	})
	return out, err
}

func (r *callRuntime) MemberIngestPosition(ctx context.Context, on extension.UserID, system extension.SourceName) (extension.MemberPosition, error) {
	if err := r.refusePositionKind(system, true); err != nil {
		return extension.MemberPosition{}, err
	}
	if !r.unattended {
		return extension.MemberPosition{}, extension.ErrAttendedIngest
	}
	member, err := r.consentedMember(ctx, on)
	if err != nil {
		return extension.MemberPosition{}, err
	}
	var saved extension.MemberPosition
	err = r.inPositionTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		saved, err = extingress.ReadPosition(ctx, tx, r.unit, string(system), member)
		return err
	})
	return saved, err
}

func (r *callRuntime) consentedMember(ctx context.Context, on extension.UserID) (ids.UUID, error) {
	member := extingress.UserUUID(on)
	if member == extingress.NoMember {
		return member, fmt.Errorf("%w: the member id is not a canonical UUID", extension.ErrInvalid)
	}
	scoped, err := r.scoped(ctx)
	if err != nil {
		return member, err
	}
	consented, err := extensionMemberConsented(scoped, r.deps.pool, r.unit, member)
	if err == nil && !consented {
		err = fmt.Errorf("%w: that member has deposited no credential with this unit", extension.ErrForbidden)
	}
	return member, err
}

func (r *callRuntime) landBatch(ctx context.Context, on extension.UserID, system extension.SourceName, recs []extension.Record, position extension.BatchPosition, perMember bool) ([]extension.Result, error) {
	if err := extension.ValidateBatch(system, recs, position); err != nil {
		return nil, err
	}
	if err := r.refusePositionKind(system, perMember); err != nil {
		return nil, err
	}
	if !r.unattended {
		return nil, extension.ErrAttendedIngest
	}
	results := make([]extension.Result, 0, len(recs))
	for _, rec := range recs {
		result, err := r.Ingest(ctx, on, rec)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (r *callRuntime) refusePositionKind(system extension.SourceName, perMember bool) error {
	declared, err := r.declaredIngress(string(system))
	if err != nil {
		return err
	}
	if memberOwned := declared.PinnedPosture != ""; memberOwned != perMember {
		return fmt.Errorf("%w: source %q keeps a position per member only if it is member-owned, and one for the stream if it is not", extension.ErrInvalid, system)
	}
	return nil
}

func (r *callRuntime) inPositionTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	scoped, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(scoped, r.deps.pool, func(tx pgx.Tx) error { return fn(scoped, tx) })
}
