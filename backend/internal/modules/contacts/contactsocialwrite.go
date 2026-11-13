// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// writeContactSocial merges a social map into the contact's links. A key the
// map does not name stays as stored. A key sent empty is removed. A key sent
// with a handle gets a new row, which tells "delete bought data" that a
// human stated the link (revertSocialHandle).
//
//nolint:unparam // all four child writers here take (ctx, tx, wsID, contactID, …); dropping wsID from the two that do not spend it would make a reader check which is which at every call site.
func writeContactSocial(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, contactID ids.ContactID, social map[string]any, converting []ids.LeadID) error {
	rows, err := contactSocialInputs(social)
	if len(social) == 0 || err != nil {
		return err
	}
	if err := refuseLeadHeldLinkedIn(ctx, tx, contactID, social, converting); err != nil {
		return err
	}
	named := slices.Collect(maps.Keys(social))
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_social WHERE contact_id = $1 AND platform = ANY($2)`, contactID, named); err != nil {
		return fmt.Errorf("clear the contact's named social rows: %w", err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_social (contact_id, platform, handle) VALUES ($1, $2, $3)`,
			contactID, row.Platform, row.Handle); err != nil {
			return fmt.Errorf("insert contact social row: %w", err)
		}
		if row.Platform == SocialLinkedIn {
			if err := fileLinkedInTwinsAtWrite(ctx, tx, contactID, row.Handle); err != nil {
				return err
			}
		}
	}
	return nil
}
