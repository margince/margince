// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

const grantRecordIDArg = "record_id"

const grantRecordTypeArg = "record_type"

const grantRecordScope = "record_type=@record_type AND record_id=@record_id"

func admitGrantCarry(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) error {
	if (kind != "contact" && kind != "company" && kind != "lead") || source.IsZero() || target.IsZero() || source == target {
		return apperrors.ErrInvalidArgument
	}
	if err := auth.Require(ctx, kind, principal.ActionUpdate); err != nil {
		return err
	}
	for _, id := range []ids.UUID{source, target} {
		if err := auth.EnsureWritable(ctx, tx, kind, id); err != nil {
			return err
		}
	}
	return nil
}

// PreviewGrantCarryTx checks both records before exposing grants that a merge would transfer.
func (s *Service) PreviewGrantCarryTx(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]authz.CarriedRecordGrant, error) {
	if err := auth.Require(ctx, kind, principal.ActionRead); err != nil {
		return nil, err
	}
	if err := admitGrantCarry(ctx, tx, kind, source, target); err != nil {
		return nil, err
	}
	retired, err := previewGrantRows(ctx, tx, kind, source)
	if err != nil {
		return nil, err
	}
	kept, err := previewGrantRows(ctx, tx, kind, target)
	if err != nil {
		return nil, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return nil, err
	}
	bySubject := map[string]grantRow{}
	for _, grant := range kept {
		bySubject[grantIdentity(grantTarget(grant, target))] = grant
	}
	var out []authz.CarriedRecordGrant
	for _, grant := range retired {
		change := authz.CarriedRecordGrant{SourceID: grant.ID, SourceVersion: grant.Version, SubjectType: grant.SubjectType, SubjectID: grant.SubjectID, Source: grantState(grant)}
		result := grant
		if prior, found := bySubject[grantIdentity(grantTarget(grant, target))]; found {
			change.TargetID, change.TargetVersion = &prior.ID, &prior.Version
			before := grantState(prior)
			change.Before = &before
			result = strongerGrant(prior, grant, now)
		}
		change.After, change.ActiveAfter = grantState(result), grantLive(result, now)
		out = append(out, change)
	}
	return out, nil
}

func previewGrantRows(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) ([]grantRow, error) {
	rows, err := tx.Query(ctx, "SELECT "+grantColumns+" FROM record_grant WHERE "+grantRecordScope+" ORDER BY id LIMIT 501", pgx.NamedArgs{grantRecordTypeArg: kind, grantRecordIDArg: id})
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (grantRow, error) { return scanGrant(row) })
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}

func grantTarget(grant grantRow, record ids.UUID) CreateGrantInput {
	return CreateGrantInput{RecordType: grant.RecordType, RecordID: record, SubjectType: grant.SubjectType, SubjectID: grant.SubjectID}
}

func grantState(grant grantRow) authz.RecordGrantState {
	return authz.RecordGrantState{Access: grant.Access, ExpiresAt: grant.ExpiresAt, GrantedBy: grant.GrantedBy, Reason: grant.Reason}
}
