// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// partnerSetUpAdapter hands the agent tool the contacts store's own partner
// set-up. A program set up over MCP and one set up in the web app then pass the
// same grants and write the same audit row. It never carries a margin tier.
type partnerSetUpAdapter struct{ store *contacts.Store }

func (a partnerSetUpAdapter) Check(ctx context.Context, companyID ids.UUID) error {
	return a.store.CheckPartnerSetUp(ctx, ids.From[ids.CompanyKind](companyID))
}

func (a partnerSetUpAdapter) SetUp(ctx context.Context, companyID ids.UUID, fields agents.PartnerSetUpFields) error {
	_, err := a.store.UpsertPartner(ctx, contacts.UpsertPartnerInput{
		CompanyID:         ids.From[ids.CompanyKind](companyID),
		PartnerRole:       fields.Role,
		RelationshipStage: fields.Stage,
		NextStep:          fields.NextStep,
		NextStepDueAt:     fields.NextStepDueAt,
		MustCreate:        true,
	})
	return err
}
