// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A website lead is not worked until its Art. 14 notice has gone out.
//
// A lead read off a company's website is deleted when its notice bounces. A
// promotion before then would copy their details onto a record the deletion
// does not reach. A claim would start work on somebody not yet told.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// websiteLeadBounceWindow is how long after its notice was sent a website lead
// stays held. A hard bounce arrives within minutes to hours; after this window
// the notice counts as delivered.
const websiteLeadBounceWindow = "72 hours"

// ErrWebsiteLeadNotYetTold refuses promoting or claiming a website lead whose
// privacy notice may still bounce.
var ErrWebsiteLeadNotYetTold = fmt.Errorf("%w: this lead was read off a website and its privacy "+
	"notice is still pending; it can be promoted or claimed once the notice has been sent for "+
	websiteLeadBounceWindow+" without bouncing", apperrors.ErrConflict)

// refuseWebsiteLeadNotYetTold refuses a lead whose notice is queued and was
// not sent, or was sent inside the bounce window.
func refuseWebsiteLeadNotYetTold(ctx context.Context, tx pgx.Tx, leadID ids.UUID) error {
	var pending bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM lead_privacy_notice n
		      LEFT JOIN comms_outbound o ON o.id = n.delivery_id
		     WHERE n.lead_id = $1 AND n.state = 'queued'
		       AND (o.sent_at IS NULL OR o.sent_at > now() - $2::interval))`,
		leadID, websiteLeadBounceWindow).Scan(&pending); err != nil {
		return fmt.Errorf("contacts: reading whether the lead's privacy notice went out: %w", err)
	}
	if pending {
		return ErrWebsiteLeadNotYetTold
	}
	return nil
}
