// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) PreviewLeadRouting(ctx context.Context, leadID ids.LeadID, cfg RoutingConfig) (RoutingDecision, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return RoutingDecision{}, err
	}
	var decision RoutingDecision
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		decision, err = planLeadRouting(ctx, ctx, tx, leadID, cfg)
		return err
	})
	return decision, err
}

func planLeadRouting(ctx, admit context.Context, tx pgx.Tx, leadID ids.LeadID, cfg RoutingConfig) (RoutingDecision, error) {
	if err := auth.Require(admit, "lead", principal.ActionUpdate); err != nil {
		return RoutingDecision{}, err
	}
	if err := auth.EnsureWritableLive(admit, tx, "lead", leadID.UUID); err != nil {
		return RoutingDecision{}, err
	}
	var currentOwner *ids.UserID
	var status string
	var facts leadRoutingFacts
	if err := tx.QueryRow(ctx, `SELECT owner_id, status, coalesce(origin, ''), coalesce(company_name, ''), coalesce(candidate_company_key, '') FROM lead WHERE id = @id`, pgx.NamedArgs{"id": leadID}).Scan(&currentOwner, &status, &facts.Origin, &facts.CompanyName, &facts.CandidateCompanyKey); err != nil {
		return RoutingDecision{}, err
	}
	if currentOwner != nil {
		return RoutingDecision{Reason: "already_owned"}, nil
	}
	if held, err := websiteLeadNoticePending(ctx, tx, leadID.UUID); err != nil || held {
		return RoutingDecision{Reason: "notice_pending"}, err
	}
	if !LeadStatus(status).Open() {
		return RoutingDecision{Reason: "terminal_status"}, nil
	}
	active, openLoad, err := ownerCapacity(ctx, tx, candidateOwners(cfg))
	if err != nil {
		return RoutingDecision{}, err
	}
	chosen, reason, assign := chooseOwner(cfg, facts, openLoad, active)
	return RoutingDecision{Assigned: assign, OwnerID: chosen, Reason: reason}, nil
}
