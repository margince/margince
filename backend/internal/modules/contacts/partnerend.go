// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const partnerRelationshipTypesField = "relationship_types"

// EndPartnerProgram retires the extension without archiving its company or
// deleting terms and commission history. A later setup can revive the same row.
func (s *Store) EndPartnerProgram(ctx context.Context, company ids.CompanyID, ifVersion *int64) error {
	if err := requirePartnerWriteGrant(ctx); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "company", company.UUID); err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "company", company.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		var live bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM partner WHERE company_id = $1 AND archived_at IS NULL)`, company).Scan(&live); err != nil {
			return err
		}
		if !live {
			return apperrors.ErrNotFound
		}
		if err := checkPartnerVersion(ctx, tx, UpsertPartnerInput{CompanyID: company, IfVersion: ifVersion}); err != nil {
			return err
		}
		retired, err := scanPartner(tx.QueryRow(ctx, `UPDATE partner SET archived_at = now() WHERE company_id = $1 AND archived_at IS NULL RETURNING `+partnerColumns, company))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		all, err := readLiveRelationshipTypes(ctx, tx, company)
		if err != nil {
			return err
		}
		core, _ := splitRelationshipTypes(all)
		desired := slices.DeleteFunc(slices.Clone(core), func(kind string) bool { return kind == relationshipTypePartner })
		if _, err := reconcileCompanyRelationshipTypes(ctx, tx, company, "manual", by, desired, false); err != nil {
			return err
		}
		before, after := map[string]any{"partner_archived_at": nil, partnerRelationshipTypesField: core}, map[string]any{"partner_archived_at": retired.ArchivedAt, partnerRelationshipTypesField: desired}
		audit, err := storekit.Audit(ctx, tx, "update", "company", company.UUID, before, after)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, company.UUID, crmcontracts.PublicEventCompanyUpdated{ChangedFields: map[string]any{eventKeyDelta: map[string]any{eventKeyPartner: nil, partnerRelationshipTypesField: desired}}})
	})
}
