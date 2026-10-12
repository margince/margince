// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// linkActivity resolves the normalized record's link refs. Every target
// is an FK argument naming a row-scoped record, so every one passes the
// visibility probe (H1) — a connector cannot plant a link to a row its
// granting human could not see.
func (s *Sink) linkActivity(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, links []datasource.EntityRef) error {
	// A backlog catching up is a parallel writer onto the same accounts, so
	// the reach is locked in the shared order before the first probe.
	var targets storekit.LastActivityTargets
	for _, link := range links {
		switch link.Type {
		case datasource.EntityContact:
			targets.Contacts = append(targets.Contacts, link.ID)
		case datasource.EntityCompany:
			targets.Companies = append(targets.Companies, link.ID)
		case datasource.EntityDeal:
			targets.Deals = append(targets.Deals, link.ID)
		}
	}
	if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	for _, link := range links {
		column, ok := map[datasource.EntityType]string{
			datasource.EntityContact: "contact_id",
			datasource.EntityCompany: "company_id",
			datasource.EntityDeal:    "deal_id",
		}[link.Type]
		if !ok {
			return fmt.Errorf("capture: activities cannot link a %s", link.Type)
		}
		if err := auth.EnsureLinkTarget(ctx, tx, string(link.Type), link.ID); err != nil {
			return fmt.Errorf("capture: link target %s %s: %w", link.Type, link.ID, err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO activity_link (activity_id, entity_type, %s)
			VALUES ($1, $2, $3)`, column),
			activityID, string(link.Type), link.ID); err != nil {
			return fmt.Errorf("capture: linking activity: %w", err)
		}
	}
	return nil
}
