// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RetireMachineEmploymentEdgesTx preserves human assertions. It retires an
// inferred employer only when every live address belongs to a sending system.
// A live phone or channel identity, which may name a human, keeps it.
func RetireMachineEmploymentEdgesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID string) (SeatEmploymentPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return SeatEmploymentPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return SeatEmploymentPage{}, err
	}
	page, edges, err := inferredEmploymentPage(ctx, tx, after, limit)
	if err != nil {
		return SeatEmploymentPage{}, err
	}
	for _, edge := range edges {
		machineOnly, err := machineOnlyContactTx(ctx, tx, edge.contact)
		if err != nil {
			return SeatEmploymentPage{}, err
		}
		if !machineOnly {
			continue
		}
		retired, err := retireInferredEmploymentEdge(ctx, tx, edge, runID, "machine_address")
		if err != nil {
			return SeatEmploymentPage{}, err
		}
		if retired {
			page.Retired++
		}
	}
	return page, nil
}

func machineOnlyContactTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return false, err
	}
	var emails []string
	var held, otherIdentity bool
	args := []any{contact}
	query := fmt.Sprintf(`SELECT legal_hold, ARRAY(
  SELECT email FROM contact_email WHERE contact_id = c.id AND archived_at IS NULL), %s
  FROM contact c WHERE id = $%d FOR UPDATE`, otherLiveIdentitySQL, len(args))
	err := tx.QueryRow(ctx, query, args...).Scan(&held, &emails, &otherIdentity)
	if err != nil {
		return false, fmt.Errorf("contacts: checking a sending system's addresses: %w", err)
	}
	return !held && !otherIdentity && len(emails) > 0 && len(technicalAddresses(emails)) == len(emails), nil
}
