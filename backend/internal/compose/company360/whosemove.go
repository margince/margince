// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyContactStrengths reads every visible contact's strength with the
// History badge's move on the mail each last sent us. A contact row and the
// badge on that contact's mail then agree on whose move it is.
func companyContactStrengths(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, projectID *ids.ProjectID,
) ([]contacts.ContactStrength, error) {
	all, err := contacts.StrengthForCompanyContacts(ctx, tx, companyID, now, projectID)
	if err != nil {
		return nil, err
	}
	heard := make([]ids.UUID, 0, len(all))
	for _, c := range all {
		if c.Strength.LastInboundActivity != nil {
			heard = append(heard, c.Strength.LastInboundActivity.UUID)
		}
	}
	moves, err := activities.MailMovesFor(ctx, tx, heard)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if at := all[i].Strength.LastInboundActivity; at != nil {
			if move, ok := moves[at.UUID]; ok {
				all[i].Strength.LastInboundMove = &move
			}
		}
	}
	return all, nil
}

// awaitsThem is whether the newest message leaves the move with the other
// side. A mail asks its badge; a call or a meeting asks its direction.
func (m lastMessage) awaitsThem() bool {
	if m.Move != nil {
		return *m.Move == crmcontracts.EmailSummaryMoveWaitingForThem
	}
	return m.Direction == string(crmcontracts.ActivityDirectionOutbound)
}

// owedByUs is whether the newest message leaves the move with us, by the same
// two rules as awaitsThem.
func (m lastMessage) owedByUs() bool {
	if m.Move != nil {
		return *m.Move == crmcontracts.EmailSummaryMoveNeedsReply
	}
	return m.Direction == string(crmcontracts.ActivityDirectionInbound)
}
