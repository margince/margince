// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (p *Provider) ReadLeadForReversal(ctx context.Context, id ids.UUID) (datasource.Record, error) {
	lead, err := p.store.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.IncludeArchived)
	if err != nil {
		return datasource.Record{}, err
	}
	if lead.Status != "promoted" || lead.ArchivedAt == nil || lead.PromotedContactId == nil {
		return datasource.Record{}, apperrors.ErrConflict
	}
	return datasource.NewRecord(datasource.EntityRef{Type: datasource.EntityLead, ID: id}, lead, lead.Version)
}
