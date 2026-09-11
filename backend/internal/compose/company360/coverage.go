// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// How well an account is covered.
//
// The contact list answers "who works here, in what order". This answers the
// reading of it a rep opens the page for: is anybody here talking to us, which
// buying roles nobody holds, and who is the warmest way in.
//
// Counted over the WHOLE account rather than a page, because a coverage figure
// taken from a page is a figure about the page and the reader cannot tell the
// two apart. The roster read this folds is the same one the 360's contacts
// section and the contact list use, so the three cannot disagree about who is
// on the account.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Coverage reads the account's coverage for the company Contacts tab.
func (s *Service) Coverage(
	ctx context.Context, companyID ids.CompanyID,
) (crmcontracts.CompanyCoverage, error) {
	// The account admission its siblings state. Coverage was the fourth company360
	// read of this shape and the one left behind: its root grant came only from
	// GetCompanyTx, one package away, where the entry-point gate cannot
	// see it.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.CompanyCoverage{}, err
	}
	now := s.now().UTC()
	out := crmcontracts.CompanyCoverage{
		AsOf:  now,
		Deals: []crmcontracts.CompanyCoverageDeal{},
	}
	active, err := s.contacts.ActiveCompanyColumns(ctx)
	if err != nil {
		return crmcontracts.CompanyCoverage{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.contacts.GetCompanyTx(ctx, tx, companyID, storekit.LiveOnly, active); err != nil {
			return err
		}
		all, err := contacts.StrengthForCompanyContacts(ctx, tx, companyID, now, nil)
		if err != nil {
			return err
		}
		out.Summary = summarise(all)
		if err := s.fillBestWayIn(ctx, tx, companyID, all, &out); err != nil {
			return err
		}
		return s.fillCommittee(ctx, tx, companyID, now, all, &out)
	})
	if err != nil {
		return crmcontracts.CompanyCoverage{}, err
	}
	return out, nil
}

// summarise counts the account by engagement state.
func summarise(all []contacts.ContactStrength) crmcontracts.CompanyCoverageSummary {
	out := crmcontracts.CompanyCoverageSummary{ContactsTotal: len(all)}
	for _, c := range all {
		switch contacts.EngagementOf(c.Strength) {
		case contacts.EngagementWaiting:
			out.Waiting++
		case contacts.EngagementAnswered:
			out.Answered++
		case contacts.EngagementNoReply:
			out.NoReply++
		case contacts.EngagementLapsed:
			out.Lapsed++
		case contacts.EngagementUntried:
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
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	all []contacts.ContactStrength, out *crmcontracts.CompanyCoverage,
) error {
	if len(all) == 0 {
		return nil
	}
	ranked := make([]contacts.ContactStrength, len(all))
	copy(ranked, all)
	contacts.RankContacts(ranked)
	best := ranked[0]
	// The top-ranked contact is no-reply only when every contact on the account
	// is, since waiting, answered and untried all outrank it — and then there is
	// no way IN to name: following up again is a decision the reader makes, not
	// a route the page recommends.
	if contacts.EngagementOf(best.Strength) == contacts.EngagementNoReply {
		return nil
	}
	identity, err := contactIdentity(ctx, tx, companyID, []ids.ContactID{best.ContactID})
	if err != nil {
		return err
	}
	who := identity[best.ContactID]
	out.BestWayIn = &crmcontracts.CompanyCoverageRoute{
		ContactId:     openapi_types.UUID(best.ContactID.UUID),
		FullName:      who.fullName,
		Title:         who.title,
		Engagement:    crmcontracts.ContactEngagement(contacts.EngagementOf(best.Strength)),
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
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time,
	all []contacts.ContactStrength, out *crmcontracts.CompanyCoverage,
) error {
	openDeals, err := s.visibleOpenDeals(ctx, tx, companyID)
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

	// Stakeholders applies the CONTACT row scope itself, so a seat whose holder
	// this caller may not see is absent from the slice rather than anonymous.
	// Counting the true total separately is what lets a gap stay honest: a
	// champion the reader cannot see is still a champion, and reporting a
	// champion gap over a partial committee names a hole that does not exist.
	total, err := seatCount(ctx, tx, selected)
	if err != nil {
		return err
	}
	committee, err := s.wireCommittee(ctx, tx, companyID, selected, now, seats, total, all)
	if err != nil {
		return err
	}
	out.Committee = &committee
	return nil
}

func (s *Service) wireCommittee(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, dealID ids.DealID,
	now time.Time, seats []deals.DealStakeholder, total int,
	all []contacts.ContactStrength,
) (crmcontracts.CompanyCoverageCommittee, error) {
	out := crmcontracts.CompanyCoverageCommittee{
		Seats:         []crmcontracts.CompanyCoverageSeat{},
		Gaps:          []string{},
		UnlistedSeats: total - len(seats),
	}
	engagement := make(map[ids.UUID]contacts.Engagement, len(all))
	for _, c := range all {
		engagement[c.ContactID.UUID] = contacts.EngagementOf(c.Strength)
	}
	contactIDs := make([]ids.ContactID, 0, len(seats))
	for _, seat := range seats {
		contactIDs = append(contactIDs, ids.From[ids.ContactKind](seat.ContactID))
	}
	identity, err := contactIdentity(ctx, tx, companyID, contactIDs)
	if err != nil {
		return crmcontracts.CompanyCoverageCommittee{}, err
	}
	// Who on our side can reach each seat, from the SAME reader the 360's
	// contacts section uses — a second route ranking would let the map and the
	// roster disagree about which colleague to ask. Absent without the
	// activity grant, because an empty set is an answer.
	var routes map[ids.UUID]crmcontracts.Company360ContactRoutes
	mayRoutes, err := mayReadRoutes(ctx)
	if err != nil {
		return crmcontracts.CompanyCoverageCommittee{}, err
	}
	if mayRoutes {
		raw := make([]ids.UUID, len(contactIDs))
		for i, id := range contactIDs {
			raw[i] = id.UUID
		}
		routes, err = contactRoutes(ctx, tx, raw, now)
		if err != nil {
			return crmcontracts.CompanyCoverageCommittee{}, err
		}
	}
	// Which of these seats the product read rather than a contact asserted, and
	// what it read them from. Kept out of deals.Stakeholders because one caller
	// needing provenance is not a reason to widen the shape every caller reads.
	marks, err := suggestedSeats(ctx, tx, dealID, seats)
	if err != nil {
		return crmcontracts.CompanyCoverageCommittee{}, err
	}
	held := map[string]bool{}
	for _, seat := range seats {
		held[seat.Role] = true
		wire := crmcontracts.CompanyCoverageSeat{
			ContactId: openapi_types.UUID(seat.ContactID),
			FullName:  identity[ids.From[ids.ContactKind](seat.ContactID)].fullName,
			Role:      seat.Role,
		}
		if state, ok := engagement[seat.ContactID]; ok {
			at := crmcontracts.ContactEngagement(state)
			wire.Engagement = &at
		}
		if route, ok := routes[seat.ContactID]; ok {
			wire.Routes = &route
		}
		if mark, ok := marks[seatKey{contact: seat.ContactID, role: seat.Role}]; ok {
			if mark.suggested {
				wire.AiSuggested = &mark.suggested
			}
			// Carried for EVERY seat, not only a read one: a colleague changing
			// a role a colleague typed is the same patch on the same row, and
			// giving the card the id only where the product happens to have
			// written the seat would make the human-typed ones the ones a
			// reader cannot correct.
			id := openapi_types.UUID(mark.relationshipID)
			wire.RelationshipId = &id
			version := mark.version
			wire.RelationshipVersion = &version
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
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
) ([]crmcontracts.CompanyCoverageDeal, error) {
	// The OBJECT grant first, and separately from the row scope: scope answers
	// WHICH deals this caller may see, never WHETHER they may ask. A caller
	// holding company, contact and relationship but not deal was being
	// served deal names and the committee sitting on them, because the read
	// reached for the scope clause and never for the grant behind it.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	dealScope, err := scopeClause(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.name FROM deal d %s ORDER BY d.updated_at DESC, d.id`,
		openDealsWhere(companyPos, dealScope)), args...)
	if err != nil {
		return nil, fmt.Errorf("company360: reading the account's open deals: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.CompanyCoverageDeal{}
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.CompanyCoverageDeal{
			DealId: openapi_types.UUID(id), Name: name,
		})
	}
	return out, rows.Err()
}

// seatCount is how many live seats the deal has, WITHOUT the contact row scope.
//
// Deliberately unscoped, and the only place in this read that is: the count
// answers "is the committee bigger than what you were shown", which is exactly
// the question the scoped read cannot answer about itself. It discloses a
// number and never a name — not which contact, not which seat, and never a
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
		return 0, fmt.Errorf("company360: counting the deal's seats: %w", err)
	}
	return total, nil
}

// seatKey identifies ONE seat, which is a contact AND a role.
//
// The table's own uniqueness key is (deal_id, contact_id, role), so a contact can
// legitimately hold two roles on one deal. Keyed by contact alone, both cards
// carried whichever row the scan returned last — so Confirm on one could patch
// the other, changing a role the reader was not looking at or colliding with
// the uniqueness index.
type seatKey struct {
	contact ids.UUID
	role    string
}

// seatMark is what a seat's own row says about where it came from.
type seatMark struct {
	suggested bool
	// relationshipID is the seat's own row, which a reader needs in order to
	// disagree with it. Confirming or changing a role is a patch on THIS row,
	// and the contact id alone cannot name it: the same contact can sit on two
	// deals, and the two seats are different rows.
	relationshipID ids.UUID
	// version is what the seat's patch sends as If-Match. Without it a reader
	// confirming a role overwrites whatever a colleague changed while the page
	// was open, and the losing write leaves no trace.
	version int64
}

// suggestedSeats reads the provenance of the committee's own rows.
//
// A seat the product read out of messages carries the proposing agent as its
// captured_by, which is the same mark every other agent write in the tree
// carries — so "which of these did we read" is answered by the row itself
// rather than by a column somebody has to remember to set.
func suggestedSeats(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, seats []deals.DealStakeholder,
) (map[seatKey]seatMark, error) {
	out := map[seatKey]seatMark{}
	if len(seats) == 0 {
		return out, nil
	}
	ids0 := make([]ids.UUID, 0, len(seats))
	for _, seat := range seats {
		ids0 = append(ids0, seat.ContactID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos, dealPos, agentPos := arg(ids0), arg(dealID), arg(proposeroles.CapturedBy)
	// The edge grant, like every other reader of this table: a seat names two
	// records, and who may learn that pair is what relationship.read governs.
	// The sibling that reads the same rows for the 360 asks for it, and a
	// second reader that did not would be the way around it.
	edgeBound, err := edgeScope(ctx, arg)
	if err != nil {
		return nil, err
	}
	// SCOPED TO THIS DEAL. Keyed by contact alone, somebody sitting on two deals
	// carried whichever row the scan happened to return last — so a seat a
	// colleague typed here could be marked as the product's reading because of
	// an unrelated deal elsewhere.
	//
	// And the SAME liveness rule the committee read applies: it excludes only
	// archived rows, so filtering ended ones here as well would drop the
	// provenance of a seat still on the board and quietly unmark it.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.contact_id, coalesce(r.role, ''), r.captured_by = $%d, r.id, r.version
		  FROM relationship r
		 WHERE r.kind = 'deal_stakeholder' AND r.contact_id = ANY($%d)
		   AND r.deal_id = $%d AND r.archived_at IS NULL
		   AND (%s)`, agentPos, contactsPos, dealPos, edgeBound), args...)
	if err != nil {
		return nil, fmt.Errorf("company360: reading the committee's provenance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key seatKey
		var mark seatMark
		if err := rows.Scan(&key.contact, &key.role, &mark.suggested,
			&mark.relationshipID, &mark.version); err != nil {
			return nil, err
		}
		out[key] = mark
	}
	return out, rows.Err()
}
