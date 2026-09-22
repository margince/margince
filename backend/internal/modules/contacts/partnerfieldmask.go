// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The partner's field masks, applied where the row meets the wire. A seat may
// read the partner register without reading what the programme pays: the
// margin tier goes out null and named in masked_fields, so the reader tells it
// from a partner nobody has tiered yet.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// partnerMaskObject is the RBAC object a partner's masks are configured under —
// the vocabulary an administrator writes in field_mask, which renaming the
// table would not rename.
const partnerMaskObject = "partner"

// partnerWithholds are the fields a mask may name on a partner, and how each is
// withheld. One deliberate act per field, which is what keeps the set finite
// enough to be offered as a catalog.
//
// The tier's reach onto the commission entry that republishes it lives in
// auth's group closure, not here: each module withholds its own field.
var partnerWithholds = map[string]func(*crmcontracts.Partner){
	"margin_tier": func(p *crmcontracts.Partner) { p.MarginTier = nil },
}

// maskPartners withholds, per row, what this reader's role does not read.
//
// No extra collector and no write-authority map: `partner` carries no owner, so
// a mask on it cannot be conditioned on write authority — one configured that
// way withholds on every row rather than lifting on rows nothing can answer for
// — and the pass costs no statement.
func maskPartners(ctx context.Context, tx pgx.Tx, page []crmcontracts.Partner) error {
	return auth.ApplyFieldMasks(ctx, tx, partnerMaskObject, page,
		func(p crmcontracts.Partner) ids.UUID { return ids.UUID(p.CompanyId) },
		partnerWithholds,
		func(p *crmcontracts.Partner, names []string) { p.MaskedFields = &names },
		nil, nil)
}

// refuseMaskedPartnerSort refuses an order over a column this caller's role
// withholds, through the refusal every such list shares. The partner list
// publishes margin_tier as a sort key, and an order is a reading of the values
// it sorts by.
func refuseMaskedPartnerSort(ctx context.Context, sort *string) error {
	return auth.RefuseMaskedSort(ctx, partnerMaskObject, sort, func(field string) bool {
		_, maskable := partnerWithholds[field]
		return maskable
	})
}

// wirePartners maps a page of rows onto the wire and masks it — the one place a
// partner row becomes a partner record, so a read added later cannot be the one
// that forgets.
func wirePartners(ctx context.Context, tx pgx.Tx, rows []partnerRow) ([]crmcontracts.Partner, error) {
	page := make([]crmcontracts.Partner, 0, len(rows))
	for _, row := range rows {
		page = append(page, wirePartner(row))
	}
	if err := maskPartners(ctx, tx, page); err != nil {
		return nil, err
	}
	return page, nil
}

// wirePartnerForCaller is wirePartners over the ONE row a single read answers
// with — or the echo of a write, which is a read too.
func wirePartnerForCaller(ctx context.Context, tx pgx.Tx, row partnerRow) (crmcontracts.Partner, error) {
	page, err := wirePartners(ctx, tx, []partnerRow{row})
	if err != nil {
		return crmcontracts.Partner{}, err
	}
	return page[0], nil
}
