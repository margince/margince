// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadProductField is the product a lead is interested in.
const leadProductField = "product_id"

// ErrLeadProductRetired marks a product the catalog no longer offers, so a
// writer that names one on the sender's behalf can land the lead without it.
var ErrLeadProductRetired = errors.New("the lead's product is retired")

// ensureLeadProduct refuses a product that is not in the live catalog. A
// product is not a row-scoped record, so any reader may name one.
func ensureLeadProduct(ctx context.Context, tx pgx.Tx, product *ids.UUID) error {
	if product == nil {
		return nil
	}
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product WHERE id = $1 AND archived_at IS NULL)`, *product).Scan(&live); err != nil {
		return fmt.Errorf("contacts: reading the lead's product: %w", err)
	}
	if !live {
		refused := httperr.Validation(leadProductField, "unknown", "choose a live product from /v1/products")
		return fmt.Errorf("%w: %w", ErrLeadProductRetired, refused)
	}
	return nil
}

// sameLeadProduct reports a patch naming the product the lead already holds.
func sameLeadProduct(current crmcontracts.Lead, product *ids.UUID) bool {
	return product != nil && current.ProductId != nil && ids.UUID(*current.ProductId) == *product
}

// setLeadReferences patches the records a lead points at that take no
// normalising: its company routing key, project and product. The checks that
// they exist ran in admitLeadReferences.
func setLeadReferences(p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) {
	if in.CandidateCompanyKey != nil {
		p.Set("candidate_company_key", current.CandidateCompanyKey, *in.CandidateCompanyKey)
	}
	if in.ProjectID != nil {
		p.Set("project_id", current.ProjectId, *in.ProjectID)
	}
	if in.ProductID != nil {
		p.Set(leadProductField, current.ProductId, *in.ProductID)
	}
}
