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

// LinkLeadCompany links a lead to a company and changes nothing else. Someone is
// a lead or a contact, so the contact at that company comes only from promotion.
func (s *Store) LinkLeadCompany(ctx context.Context, id ids.LeadID, companyID ids.CompanyID) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := httperr.RequireBodyID(leadCompanyIDColumn, companyID.UUID); err != nil {
		return crmcontracts.Lead{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		out, err = linkLeadCompanyTx(ctx, tx, id, companyID, active)
		return err
	})
	return out, s.AskHolderToShare(ctx, err)
}

func linkLeadCompanyTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, companyID ids.CompanyID, active []fieldcatalog.Column,
) (crmcontracts.Lead, error) {
	// Asked before any lock, so a company the caller cannot open is never locked for them.
	visible, err := auth.VisibleTo(ctx, tx, companyEntity, companyID.UUID)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if !visible {
		return crmcontracts.Lead{}, apperrors.ErrNotFound
	}
	// The name lock, then the company, then the lead: a company merge's order.
	// The name lock keeps a second company of the matched name out until commit.
	if err := lockCompanyNameWrites(ctx, tx); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := ensureLinkableCompany(ctx, tx, companyID); err != nil {
		return crmcontracts.Lead{}, err
	}
	lead, lock, err := linkableLead(ctx, tx, id)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	linked, err := leadLinkedCompany(ctx, tx, id)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if linked != nil && *linked == companyID {
		return readLead(ctx, tx, id, storekit.LiveOnly, active)
	}
	if err := ensureCurrentMatch(ctx, tx, lead, companyID); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := writeLeadCompanyLink(ctx, tx, lock, id, linked, companyID); err != nil {
		return crmcontracts.Lead{}, err
	}
	return readLead(ctx, tx, id, storekit.LiveOnly, active)
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

// linkableLead locks and reads the lead a link is about. It refuses a lead the
// caller may not change, one that is closed, one its privacy notice holds, and
// one with neither name nor email.
func linkableLead(ctx context.Context, tx pgx.Tx, id ids.LeadID) (crmcontracts.Lead, storekit.RowLock, error) {
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		return crmcontracts.Lead{}, storekit.RowLock{}, err
	}
	lock, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Lead{}, storekit.RowLock{}, err
	}
	lead, err := readLead(ctx, tx, id, storekit.LiveOnly, nil)
	if err != nil {
		return crmcontracts.Lead{}, storekit.RowLock{}, err
	}
	if err := refuseWebsiteLeadNotYetTold(ctx, tx, id.UUID); err != nil {
		return crmcontracts.Lead{}, storekit.RowLock{}, err
	}
	if leadIdentityName(lead) == "" {
		return crmcontracts.Lead{}, storekit.RowLock{}, &PromoteNeedsIdentityError{}
	}
	return lead, lock, nil
}

// ensureLinkableCompany refuses a company that is archived, merged away or out
// of the caller's sight, all as 404. It holds the row FOR UPDATE, as a promotion does.
func ensureLinkableCompany(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	live, err := companyIsLive(ctx, tx, companyID)
	if err != nil {
		return err
	}
	if !live {
		return apperrors.ErrNotFound
	}
	return auth.EnsureLinkTarget(ctx, tx, companyEntity, companyID.UUID)
}

// writeLeadCompanyLink sets the link on the locked lead, with its audit row and
// lead.updated event.
func writeLeadCompanyLink(ctx context.Context, tx pgx.Tx, lock storekit.RowLock, id ids.LeadID,
	linked *ids.CompanyID, companyID ids.CompanyID,
) error {
	p := storekit.NewPatch()
	p.Set(leadCompanyIDColumn, untypedPtr(linked), companyID.UUID)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("link the lead to its company: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "lead", id.UUID, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit the lead's company link: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadUpdated{ChangedFields: p.After()}); err != nil {
		return fmt.Errorf("emit lead.updated for the company link: %w", err)
	}
	return nil
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
// the lead's linked company. A company the promoter cannot open or attach to adds
// no edge and does not stop the promotion.
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
	var name *string
	if typed, ok := renamed.(string); ok {
		name = &typed
	}
	stillNamed, err := namesCompany(ctx, tx, *linked, name)
	if err != nil {
		return err
	}
	if !stillNamed {
		p.Set(leadCompanyIDColumn, linked.UUID, nil)
	}
	return nil
}

// namesCompany reports whether a lead's company name names a company, compared
// the way the match offer compares. A nil or blank name names none.
func namesCompany(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, name *string) (bool, error) {
	var named bool
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(`+exactNameKeySQL("display_name")+` = `+exactNameKeySQL("$2::text")+`, false)
		  FROM company WHERE id = $1`, companyID, name).Scan(&named); err != nil {
		return false, fmt.Errorf("compare a lead's company name with its linked company: %w", err)
	}
	return named, nil
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
