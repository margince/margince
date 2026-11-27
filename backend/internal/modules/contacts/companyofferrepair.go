// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OfferDuplicatePage reports inspected companies and cleared duplicate descriptions.
type OfferDuplicatePage struct {
	Scanned []ids.UUID
	Changed int
}

const siteOfferDuplicateScope = `c.description IS NOT NULL AND EXISTS (
 SELECT 1 FROM company_profile_field f WHERE f.company_id = c.id
 AND f.field = 'offer_summary' AND f.source = 'site_read')`

// ClearSiteOfferDuplicatesTx clears proven site-read mirrors while preserving user authorship.
func ClearSiteOfferDuplicatesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (OfferDuplicatePage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return OfferDuplicatePage{}, err
	}
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return OfferDuplicatePage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id FROM company c WHERE c.id > @afterCompanyID AND `+siteOfferDuplicateScope+` ORDER BY c.id LIMIT @batchLimit`, pgx.NamedArgs{"afterCompanyID": after, "batchLimit": limit})
	if err != nil {
		return OfferDuplicatePage{}, fmt.Errorf("page company offer duplicates: %w", err)
	}
	page := OfferDuplicatePage{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return OfferDuplicatePage{}, err
		}
		page.Scanned = append(page.Scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return OfferDuplicatePage{}, err
	}
	for _, id := range page.Scanned {
		changed, err := clearSiteOfferDuplicate(ctx, tx, ids.From[ids.CompanyKind](id))
		if err != nil {
			return OfferDuplicatePage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func clearSiteOfferDuplicate(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (bool, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return false, err
	}
	lock, err := storekit.LockRow(ctx, tx, entityCompany, id.UUID, storekit.IncludeArchived)
	if err != nil {
		return false, err
	}
	held, err := descriptionHeldByHuman(ctx, tx, id)
	if err != nil || held {
		return false, err
	}
	var description, offer string
	var siteOwned bool
	err = tx.QueryRow(ctx, `SELECT COALESCE(c.description, ''), COALESCE(f.value, ''),
 COALESCE((SELECT fp.source = 'site_read' AND NOT `+storekit.StampForAUserSQL("fp.captured_by")+`
 FROM field_provenance fp WHERE fp.object_type = 'company' AND fp.object_id = c.id
 AND fp.field_name = 'description' AND fp.value IS NULL ORDER BY fp.captured_at DESC, fp.id DESC LIMIT 1), false)
 FROM company c LEFT JOIN company_profile_field f ON f.company_id = c.id AND f.field = 'offer_summary'
 AND f.source = 'site_read' AND NOT `+storekit.StampForAUserSQL("f.captured_by")+`
 WHERE c.id = @companyID`, pgx.NamedArgs{"companyID": id}).Scan(&description, &offer, &siteOwned)
	if err != nil {
		return false, err
	}
	if !siteOwned || offer == "" || description != headerLine(offer) {
		return false, nil
	}
	patch := storekit.NewPatch()
	patch.Set("description", description, nil)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, err
	}
	if err := recordCompanyUpdate(ctx, tx, id, patch, UpdateCompanyInput{}, ""); err != nil {
		return false, err
	}
	return true, nil
}

// CountSiteOfferDuplicates bounds the repair estimate to the supplied cap plus one.
func CountSiteOfferDuplicates(ctx context.Context, q datatask.Querier, limit int64) (int64, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM company c WHERE `+siteOfferDuplicateScope+` LIMIT @estimateCap) candidates`, pgx.NamedArgs{"estimateCap": limit + 1}).Scan(&count)
	return count, err
}
