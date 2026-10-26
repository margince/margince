// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A lead's link to the company a human confirmed it belongs to. This file holds
// the offer a lead read makes and the confirm that writes the link. It also
// holds what an edit of the company name and a promotion do to the link.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const leadCompanyIDColumn = "company_id"

// LinkLeadCompany links a lead to a company and lands it on a contact employed
// there, in one transaction. The contact is the one promotion would land on, but
// the lead is not promoted: its status, owner and place on the lead list stay.
func (s *Store) LinkLeadCompany(ctx context.Context, id ids.LeadID, companyID ids.CompanyID) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return crmcontracts.Lead{}, err
	}
	// The employment edge the link writes asks both, so a caller without them
	// is refused here rather than after the contact was found or made.
	if err := requireRelationshipCreate(ctx, anchorContact); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := httperr.RequireBodyID(leadCompanyIDColumn, companyID.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	// Restarts when the contact the lead's email matches moved, as QualifyLead does.
	for range promoteAttempts {
		var out crmcontracts.Lead
		err = s.tx(ctx, func(tx pgx.Tx) error {
			candidate, err := s.promoteCandidate(ctx, tx, id)
			if err != nil {
				return err
			}
			out, err = s.linkLeadCompanyTx(ctx, tx, id, companyID, by, candidate, active)
			return err
		})
		if _, moved := errors.AsType[*promoteTargetMovedError](err); !moved {
			return out, s.AskHolderToShare(ctx, err)
		}
	}
	return crmcontracts.Lead{}, apperrors.ErrConflict
}

func (s *Store) linkLeadCompanyTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, companyID ids.CompanyID, by string,
	candidate ids.ContactID, active []fieldcatalog.Column,
) (crmcontracts.Lead, error) {
	// Asked before any lock, so a company the caller cannot open is never locked for them.
	visible, err := auth.VisibleTo(ctx, tx, companyEntity, companyID.UUID)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if !visible {
		return crmcontracts.Lead{}, apperrors.ErrNotFound
	}
	lanes, err := s.lockLeadForContact(ctx, tx, id, candidate, &companyID)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	lead, err := linkableLead(ctx, tx, id)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := ensureLinkableCompany(ctx, tx, companyID); err != nil {
		return crmcontracts.Lead{}, err
	}
	if done, err := s.alreadyLinked(ctx, tx, lead, companyID); err != nil || done {
		if err != nil {
			return crmcontracts.Lead{}, err
		}
		return readLead(ctx, tx, id, storekit.LiveOnly, active)
	}
	if err := ensureCurrentMatch(ctx, tx, lead, companyID); err != nil {
		return crmcontracts.Lead{}, err
	}
	var merged bool
	contactID, mergeFields, err := s.promoteTarget(ctx, tx, lead, by, candidate, lanes, &merged)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := s.ensureEmployment(ctx, tx, contactID, companyID, deref(lead.Title), manualSource); err != nil {
		return crmcontracts.Lead{}, err
	}
	auditID, err := writeLeadCompanyLink(ctx, tx, id, companyID)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	contact, err := readContact(ctx, tx, contactID, storekit.LiveOnly, nil)
	if err != nil {
		return crmcontracts.Lead{}, fmt.Errorf("read the contact a lead was linked through: %w", err)
	}
	if err := emitLeadContactEvent(ctx, tx, auditID, contact, contactID, merged, mergeFields); err != nil {
		return crmcontracts.Lead{}, err
	}
	return readLead(ctx, tx, id, storekit.LiveOnly, active)
}

// alreadyLinked reports a repeat of a link that changed nothing: the lead names
// this company and the contact its email matches is already employed there.
func (s *Store) alreadyLinked(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, companyID ids.CompanyID) (bool, error) {
	linked, err := leadLinkedCompany(ctx, tx, ids.From[ids.LeadKind](ids.UUID(lead.Id)))
	if err != nil || linked == nil || *linked != companyID {
		return false, err
	}
	match, err := s.previewTarget(ctx, tx, lead)
	if err != nil || match.Decision != DecisionExactCollision {
		return false, err
	}
	return holdsLiveEmployment(ctx, tx, match.ContactID, companyID)
}

// ensureCurrentMatch refuses a company that is not the lead's match as the
// locked lead reads now. A stale offer, a second company of the same name or
// an id the offer never named are all refused by name.
func ensureCurrentMatch(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, companyID ids.CompanyID) error {
	match, err := uniqueCompanyMatch(ctx, tx, deref(lead.CompanyName))
	if err != nil {
		return err
	}
	if match == nil || ids.UUID(match.Id) != companyID.UUID {
		return httperr.Validation(leadCompanyIDColumn, "not_the_lead_match",
			"company_id is not the one company the lead's company name names; reload the lead and confirm the company it offers")
	}
	return nil
}

// linkableLead reads the lead a link is about under its lock. It refuses a lead
// the caller may not change, and one that is closed. It also refuses one its
// privacy notice holds, and one with neither name nor email.
func linkableLead(ctx context.Context, tx pgx.Tx, id ids.LeadID) (crmcontracts.Lead, error) {
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	lead, err := readLead(ctx, tx, id, storekit.LiveOnly, nil)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := refuseWebsiteLeadNotYetTold(ctx, tx, id.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	if leadIdentityName(lead) == "" {
		return crmcontracts.Lead{}, &PromoteNeedsIdentityError{}
	}
	return lead, nil
}

// ensureLinkableCompany refuses a company that is archived, merged away or out
// of the caller's sight, all as 404. EnsureLinkTarget holds the row until commit.
func ensureLinkableCompany(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, companyID.UUID); err != nil {
		return err
	}
	live, err := companyIsLive(ctx, tx, companyID)
	if err != nil {
		return err
	}
	if !live {
		return apperrors.ErrNotFound
	}
	return nil
}

// writeLeadCompanyLink sets the link and answers the lead's audit row. The
// contact stays named by its converted_from_lead_id; from_contact_id keeps
// meaning the contact a lead was created from.
func writeLeadCompanyLink(ctx context.Context, tx pgx.Tx, id ids.LeadID, companyID ids.CompanyID) (ids.UUID, error) {
	lock, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly)
	if err != nil {
		return ids.UUID{}, err
	}
	linked, err := leadLinkedCompany(ctx, tx, id)
	if err != nil {
		return ids.UUID{}, err
	}
	p := storekit.NewPatch()
	p.Set(leadCompanyIDColumn, untypedPtr(linked), companyID.UUID)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return ids.UUID{}, fmt.Errorf("link the lead to its company: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "lead", id.UUID, p.Before(), p.After())
	if err != nil {
		return ids.UUID{}, fmt.Errorf("audit the lead's company link: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadUpdated{ChangedFields: p.After()}); err != nil {
		return ids.UUID{}, fmt.Errorf("emit lead.updated for the company link: %w", err)
	}
	return auditID, nil
}

// leadLinkedCompany is the company a lead row is linked to, or nil. Read off
// the row: the wire lead withholds a company its reader cannot open.
func leadLinkedCompany(ctx context.Context, tx pgx.Tx, id ids.LeadID) (*ids.CompanyID, error) {
	var company *ids.CompanyID
	err := tx.QueryRow(ctx, `SELECT company_id FROM lead WHERE id = $1`, id).Scan(&company)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // a lead with no row has no linked company, the same answer as an unlinked one
	}
	if err != nil {
		return nil, fmt.Errorf("read the company a lead is linked to: %w", err)
	}
	return company, nil
}

// carryLeadCompany gives the contact a lead was promoted into its employment at
// the lead's linked company, through the writer the link itself uses. A company
// the promoter cannot open or attach to adds no edge and does not stop the promotion.
func (s *Store) carryLeadCompany(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID, title string) error {
	company, err := leadLinkedCompany(ctx, tx, leadID)
	if err != nil || company == nil {
		return err
	}
	if !auth.Allows(ctx, "relationship", principal.ActionCreate) || !auth.Allows(ctx, "contact", principal.ActionUpdate) {
		return nil
	}
	visible, err := auth.VisibleTo(ctx, tx, companyEntity, company.UUID)
	if err != nil || !visible {
		return err
	}
	live, err := companyIsLive(ctx, tx, *company)
	if err != nil || !live {
		return err
	}
	return s.ensureEmployment(ctx, tx, contactID, *company, title, manualSource)
}

// readLeadOffered is the lead GET /leads/{id} answers: the lead, with the
// company match it offers.
func readLeadOffered(ctx context.Context, tx pgx.Tx, id ids.LeadID, archived storekit.ArchivedFilter,
	active []fieldcatalog.Column,
) (crmcontracts.Lead, error) {
	lead, err := readLead(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	return lead, attachCompanyMatch(ctx, tx, &lead)
}

// attachCompanyMatch offers the lead's company match while the lead links none.
func attachCompanyMatch(ctx context.Context, tx pgx.Tx, lead *crmcontracts.Lead) error {
	if lead.ArchivedAt != nil {
		return nil
	}
	linked, err := leadLinkedCompany(ctx, tx, ids.From[ids.LeadKind](ids.UUID(lead.Id)))
	if err != nil || linked != nil {
		return err
	}
	lead.CompanyMatch, err = uniqueCompanyMatch(ctx, tx, deref(lead.CompanyName))
	return err
}

// uniqueCompanyMatch is the one live company the reader can open whose folded
// name equals companyName, or nil when none or more than one does. The offer
// and the link both ask it, so the link confirms what the offer showed.
func uniqueCompanyMatch(ctx context.Context, tx pgx.Tx, companyName string) (*crmcontracts.LeadCompanyMatch, error) {
	if strings.TrimSpace(companyName) == "" || !auth.ReadGranted(ctx, "company") {
		return nil, nil //nolint:nilnil // no name, or no company read, means no match to offer, not a failure
	}
	args := []any{companyName}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, companyEntity, "", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	rows, err := tx.Query(ctx, `
		SELECT id, display_name FROM company
		 WHERE archived_at IS NULL AND merged_into_id IS NULL
		   AND `+exactNameKeySQL("display_name")+` = `+exactNameKeySQL("$1")+` AND `+scope+`
		 ORDER BY id LIMIT 2`, args...)
	if err != nil {
		return nil, fmt.Errorf("look for the company a lead names: %w", err)
	}
	defer rows.Close()
	var matches []crmcontracts.LeadCompanyMatch
	for rows.Next() {
		var match crmcontracts.LeadCompanyMatch
		if err := rows.Scan(&match.Id, &match.Name); err != nil {
			return nil, fmt.Errorf("read the company a lead names: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the companies a lead names: %w", err)
	}
	if len(matches) != 1 {
		return nil, nil //nolint:nilnil // none or several companies is no unique match, which is a valid answer
	}
	return &matches[0], nil
}

// unlinkRenamedCompany drops the company link when an edit of the lead's
// company name stops naming the linked company. A name that still names it,
// compared the way the match offer compares, keeps the link.
func unlinkRenamedCompany(ctx context.Context, tx pgx.Tx, id ids.LeadID, p *storekit.Patch) error {
	renamed, touched := p.Moved()[leadCompanyColumn]
	if !touched {
		return nil
	}
	linked, err := leadLinkedCompany(ctx, tx, id)
	if err != nil || linked == nil {
		return err
	}
	var stillNamed bool
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(`+exactNameKeySQL("display_name")+` = `+exactNameKeySQL("$2::text")+`, false)
		  FROM company WHERE id = $1`, linked, renamed).Scan(&stillNamed); err != nil {
		return fmt.Errorf("compare the lead's new company name with its linked company: %w", err)
	}
	if !stillNamed {
		p.Set(leadCompanyIDColumn, linked.UUID, nil)
	}
	return nil
}

// withholdUnreadableCompanies blanks company_id on every lead whose company the
// caller cannot open, as withholdUnreadableSourceContacts does for the contact.
func withholdUnreadableCompanies(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	var companyIDs []ids.UUID
	for _, l := range leads {
		if l.CompanyId != nil {
			companyIDs = append(companyIDs, ids.UUID(*l.CompanyId))
		}
	}
	if len(companyIDs) == 0 {
		return nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, companyEntity, companyIDs)
	if err != nil {
		return err
	}
	for i := range leads {
		if leads[i].CompanyId != nil && !visible[ids.UUID(*leads[i].CompanyId)] {
			leads[i].CompanyId = nil
		}
	}
	return nil
}
