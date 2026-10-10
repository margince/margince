// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The partner's field masks, applied where the row meets the wire. A seat may
// read the partner register without reading what the programme pays: the
// margin tier goes out null and named in masked_fields, so the reader tells it
// from a partner nobody has tiered yet.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// partnerMaskObject is the RBAC object a partner's masks are configured under —
// the vocabulary an administrator writes in field_mask, which renaming the
// table would not rename.
const partnerMaskObject = "partner"

// partnerFieldMarginTier is the WIRE field a mask names. The sort catalog and
// the audit before-image spell the same word for the column, and the three stay
// separate constants: one name they happen to share is not one vocabulary.
const partnerFieldMarginTier = "margin_tier"

// partnerFieldNextStepDue is the wire field a null clears; the sort catalog and
// the audit image spell it the same way and stay their own vocabularies.
const partnerFieldNextStepDue = "next_step_due_at"

// partnerWithholds are the fields a mask may name on a partner, and how each is
// withheld. One deliberate act per field, which is what keeps the set finite
// enough to be offered as a catalog.
//
// The tier's reach onto the commission entry that republishes it lives in
// auth's group closure, not here: each module withholds its own field.
var partnerWithholds = map[string]func(*crmcontracts.Partner){
	partnerFieldMarginTier: func(p *crmcontracts.Partner) { p.MarginTier = nil },
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

// clearsTheCallerMayMake drops, from the fields a body sent as null, those this
// reader's role withholds. A seat that cannot read the tier is handed a null
// there and sends it back with every unrelated edit. Honouring it would let the
// mask wipe what it hides.
func clearsTheCallerMayMake(ctx context.Context, tx pgx.Tx, cleared []string) ([]string, error) {
	if len(cleared) == 0 {
		return nil, nil
	}
	probe := []crmcontracts.Partner{{}}
	if err := maskPartners(ctx, tx, probe); err != nil {
		return nil, err
	}
	if probe[0].MaskedFields == nil {
		return cleared, nil
	}
	return slices.DeleteFunc(slices.Clone(cleared), func(field string) bool {
		return slices.Contains(*probe[0].MaskedFields, field)
	}), nil
}

// partnerClearable are the nullable fields the upsert sets to NULL. The write is
// one upsert statement, not a storekit.Patch, so ApplyClears has no patch to act
// on. This set and its refusal are the same contract. gate_metrics clears both
// numbers it carries.
var partnerClearable = []string{
	partnerFieldMarginTier, "next_step", partnerFieldNextStepDue, "served_segments", "gate_metrics",
}

// refuseUnclearablePartnerFields answers an explicit null on a field this
// record cannot set to nothing, so it is not accepted and silently dropped.
func refuseUnclearablePartnerFields(cleared []string) error {
	for _, field := range cleared {
		if !slices.Contains(partnerClearable, field) {
			return &storekit.NotClearableError{Field: field}
		}
	}
	return nil
}

// refuseBlankServedSegments refuses a segment with no visible text. A null
// member decodes to the empty string, so it is refused here too instead of
// stored as a word beside the real ones.
func refuseBlankServedSegments(segments *[]string) error {
	if segments == nil {
		return nil
	}
	for _, segment := range *segments {
		if !values.HasVisibleText(segment) {
			return httperr.Validation("served_segments", "invalid",
				"served_segments holds only words; remove an empty or null entry")
		}
	}
	return nil
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

// wirePartners maps a page of rows onto the wire and masks it, so a read that
// reaches for it cannot be the one that forgets the withhold.
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
