// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// How well an account is covered.
//
// The contact list answers "who works here, in what order". This answers the
// reading of it a rep opens the page for: is anybody here talking to us, which
// buying roles nobody holds, and who is the warmest way in.
//
// Counted over the WHOLE account rather than a page, because a coverage figure
// taken from a page is a figure about the page and the reader cannot tell the
// two apart. The roster read this folds is the same one the 360's people
// section and the contact list use, so the three cannot disagree about who is
// on the account.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Coverage reads the account's coverage for the company People tab.
func (s *Service) Coverage(
	ctx context.Context, orgID ids.OrganizationID,
) (crmcontracts.OrganizationCoverage, error) {
	now := s.now().UTC()
	out := crmcontracts.OrganizationCoverage{
		AsOf:  now,
		Deals: []crmcontracts.OrganizationCoverageDeal{},
	}
	active, err := s.people.ActiveOrganizationColumns(ctx)
	if err != nil {
		return crmcontracts.OrganizationCoverage{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.people.GetOrganizationTx(ctx, tx, orgID, storekit.LiveOnly, active); err != nil {
			return err
		}
		all, err := people.StrengthForOrgContacts(ctx, tx, orgID, now)
		if err != nil {
			return err
		}
		out.Summary = summarise(all)
		if err := s.fillBestWayIn(ctx, tx, orgID, all, &out); err != nil {
			return err
		}
		return s.fillCommittee(ctx, tx, orgID, now, all, &out)
	})
	if err != nil {
		return crmcontracts.OrganizationCoverage{}, err
	}
	return out, nil
}

// summarise counts the account by engagement state.
func summarise(all []people.ContactStrength) crmcontracts.OrganizationCoverageSummary {
	out := crmcontracts.OrganizationCoverageSummary{ContactsTotal: len(all)}
	for _, c := range all {
		switch people.EngagementOf(c.Strength) {
		case people.EngagementAnswered:
			out.Answered++
		case people.EngagementNoReply:
			out.NoReply++
		case people.EngagementUntried:
			out.Untried++
		}
	}
	return out
}

// fillBestWayIn names the contact most worth writing to.
//
// The SAME ranking the contact list opens on, by calling the same function: a
// "best way in" chosen by its own rule would name somebody the list does not
// put first, on the one screen that shows both.
func (s *Service) fillBestWayIn(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	all []people.ContactStrength, out *crmcontracts.OrganizationCoverage,
) error {
	if len(all) == 0 {
		return nil
	}
	ranked := make([]people.ContactStrength, len(all))
	copy(ranked, all)
	people.RankContacts(ranked)
	best := ranked[0]
	// Nobody has answered and nobody is untried only when the account is all
	// no-reply, and then there is no way IN to name — following up again is a
	// decision the reader makes, not a route the page recommends.
	if people.EngagementOf(best.Strength) == people.EngagementNoReply {
		return nil
	}
	identity, err := contactIdentity(ctx, tx, orgID, []ids.PersonID{best.PersonID})
	if err != nil {
		return err
	}
	who := identity[best.PersonID]
	out.BestWayIn = &crmcontracts.OrganizationCoverageRoute{
		PersonId:      openapi_types.UUID(best.PersonID.UUID),
		FullName:      who.fullName,
		Title:         who.title,
		Engagement:    crmcontracts.ContactEngagement(people.EngagementOf(best.Strength)),
		LastInboundAt: best.Strength.LastInbound,
	}
	return nil
}

// fillCommittee reads the open deals and the selected one's buying committee.
//
// A caller refused the deal or relationship grant gets `committee_read: false`
// and no committee at all — not an empty one. An empty committee is an answer
// ("nobody holds a role"), and giving that answer to somebody who was not
// allowed to ask is the disclosure inverted.
func (s *Service) fillCommittee(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time,
	all []people.ContactStrength, out *crmcontracts.OrganizationCoverage,
) error {
	openDeals, err := s.visibleOpenDeals(ctx, tx, orgID)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	out.Deals = openDeals
	if len(openDeals) == 0 {
		// No open deal is a complete answer, not a refused one: there is no
		// committee to read because there is no deal to hold one.
		out.Completeness.CommitteeRead = true
		return nil
	}
	selected := ids.From[ids.DealKind](ids.UUID(openDeals[0].DealId))
	out.SelectedDealId = &openDeals[0].DealId

	seats, err := deals.Stakeholders(ctx, tx, selected, now)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	out.Completeness.CommitteeRead = true

	// Stakeholders applies the PERSON row scope itself, so a seat whose holder
	// this caller may not see is absent from the slice rather than anonymous.
	// Counting the true total separately is what lets a gap stay honest: a
	// champion the reader cannot see is still a champion, and reporting a
	// champion gap over a partial committee names a hole that does not exist.
	total, err := seatCount(ctx, tx, selected)
	if err != nil {
		return err
	}
	committee, err := s.wireCommittee(ctx, tx, orgID, now, seats, total, all)
	if err != nil {
		return err
	}
	out.Committee = &committee
	return nil
}

func (s *Service) wireCommittee(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time,
	seats []deals.DealStakeholder, total int, all []people.ContactStrength,
) (crmcontracts.OrganizationCoverageCommittee, error) {
	out := crmcontracts.OrganizationCoverageCommittee{
		Seats:         []crmcontracts.OrganizationCoverageSeat{},
		Gaps:          []string{},
		UnlistedSeats: total - len(seats),
	}
	engagement := make(map[ids.UUID]people.Engagement, len(all))
	for _, c := range all {
		engagement[c.PersonID.UUID] = people.EngagementOf(c.Strength)
	}
	personIDs := make([]ids.PersonID, 0, len(seats))
	for _, seat := range seats {
		personIDs = append(personIDs, ids.From[ids.PersonKind](seat.PersonID))
	}
	identity, err := contactIdentity(ctx, tx, orgID, personIDs)
	if err != nil {
		return crmcontracts.OrganizationCoverageCommittee{}, err
	}
	// Who on our side can reach each seat, from the SAME reader the 360's
	// people section uses — a second route ranking would let the map and the
	// roster disagree about which colleague to ask. Absent without the
	// activity grant, because an empty set is an answer.
	var routes map[ids.UUID]crmcontracts.Organization360ContactRoutes
	mayRoutes, err := mayReadRoutes(ctx)
	if err != nil {
		return crmcontracts.OrganizationCoverageCommittee{}, err
	}
	if mayRoutes {
		raw := make([]ids.UUID, len(personIDs))
		for i, id := range personIDs {
			raw[i] = id.UUID
		}
		routes, err = contactRoutes(ctx, tx, raw, now)
		if err != nil {
			return crmcontracts.OrganizationCoverageCommittee{}, err
		}
	}
	held := map[string]bool{}
	for _, seat := range seats {
		held[seat.Role] = true
		wire := crmcontracts.OrganizationCoverageSeat{
			PersonId: openapi_types.UUID(seat.PersonID),
			FullName: identity[ids.From[ids.PersonKind](seat.PersonID)].fullName,
			Role:     seat.Role,
		}
		if state, ok := engagement[seat.PersonID]; ok {
			at := crmcontracts.ContactEngagement(state)
			wire.Engagement = &at
		}
		if route, ok := routes[seat.PersonID]; ok {
			wire.Routes = &route
		}
		out.Seats = append(out.Seats, wire)
	}
	// Gaps only over a committee read in full — see the field's own contract.
	if out.UnlistedSeats == 0 {
		for _, role := range dealrole.Critical {
			if !held[role] {
				out.Gaps = append(out.Gaps, role)
			}
		}
	}
	return out, nil
}

// visibleOpenDeals lists the account's open deals.
//
// It builds its WHERE through openDealsWhere, which the deals section and the
// suggestion rules also build theirs from — so a condition added there reaches
// this read too, and this card cannot name a deal the deals card would refuse
// to show.
func (s *Service) visibleOpenDeals(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
) ([]crmcontracts.OrganizationCoverageDeal, error) {
	// The OBJECT grant first, and separately from the row scope: scope answers
	// WHICH deals this caller may see, never WHETHER they may ask. A caller
	// holding organization, person and relationship but not deal was being
	// served deal names and the committee sitting on them, because the read
	// reached for the scope clause and never for the grant behind it.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	dealScope, err := scopeClause(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.name FROM deal d %s ORDER BY d.updated_at DESC, d.id`,
		openDealsWhere(orgPos, dealScope)), args...)
	if err != nil {
		return nil, fmt.Errorf("org360: reading the account's open deals: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.OrganizationCoverageDeal{}
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.OrganizationCoverageDeal{
			DealId: openapi_types.UUID(id), Name: name,
		})
	}
	return out, rows.Err()
}

// seatCount is how many live seats the deal has, WITHOUT the person row scope.
//
// Deliberately unscoped, and the only place in this read that is: the count
// answers "is the committee bigger than what you were shown", which is exactly
// the question the scoped read cannot answer about itself. It discloses a
// number and never a name — not which person, not which seat, and never a
// pair. What it can tell a reader is that the committee is bigger than the
// list they were given, which is the fact the page needs in order NOT to
// print "nobody is champion" over a committee it could not read whole.
//
// The alternative is worse in the direction that matters: suppressing the
// number as well would leave the page unable to tell an empty committee from
// a partial one, and it would then have to either name a gap that may not
// exist or never name one at all.
//
// It excludes EXACTLY what the scoped read excludes — archived seats and
// nothing else. Filtering more here (an ended_at arm the stakeholder read does
// not carry) makes the total smaller than the slice it is compared against, and
// unlisted_seats goes negative on a seat that has ended but not been archived.
func seatCount(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (int, error) {
	var total int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM relationship
		 WHERE kind = 'deal_stakeholder' AND deal_id = $1
		   AND archived_at IS NULL`, dealID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("org360: counting the deal's seats: %w", err)
	}
	return total, nil
}
