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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
	match, err := leadCompanyMatch(ctx, tx, lead)
	if err != nil {
		return err
	}
	if match == nil || ids.UUID(match.Id) != companyID.UUID {
		return httperr.Validation(leadCompanyIDColumn, "not_the_lead_match",
			"company_id is not the company the lead's domain or company name names; reload the lead and confirm the company it offers")
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

// carryLeadIndustry writes the lead's typed industry to its linked company when that has
// none. A company the promoter may not change keeps its value and does not stop the promotion.
func (s *Store) carryLeadIndustry(ctx context.Context, tx pgx.Tx, company *ids.CompanyID, industry *crmcontracts.LeadIndustry) error {
	code, err := leadIndustryCarry(ctx, tx, company, industry)
	if err != nil || code == nil {
		return err
	}
	_, err = s.updateCompanyInTx(ctx, tx, *company, UpdateCompanyInput{Industry: code}, nil)
	return err
}

// leadIndustryCarry is the code a promotion writes to the lead's linked company, or nil.
// The promotion already holds that company's row lock (lockLeadForContact), taken before the lead's.
func leadIndustryCarry(ctx context.Context, tx pgx.Tx, company *ids.CompanyID, industry *crmcontracts.LeadIndustry,
) (*string, error) {
	if company == nil || industry == nil || !auth.ReadGranted(ctx, companyEntity) ||
		!auth.Allows(ctx, companyEntity, principal.ActionUpdate) {
		return nil, nil //nolint:nilnil // nothing to carry is an answer, not a failure
	}
	err := auth.EnsureChangeable(ctx, tx, companyEntity, company.UUID)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil //nolint:nilnil // a company the promoter may not change keeps its value
	}
	if err != nil {
		return nil, err
	}
	var held *string
	err = tx.QueryRow(ctx, `SELECT industry FROM company WHERE id = $1 AND archived_at IS NULL AND merged_into_id IS NULL`,
		company).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) || held != nil {
		return nil, nil //nolint:nilnil // a gone company, or one with an industry, takes none
	}
	if err != nil {
		return nil, fmt.Errorf("read the industry of the company a lead is promoted at: %w", err)
	}
	code := string(*industry)
	return &code, nil
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
	lead.CompanyMatch, err = leadCompanyMatch(ctx, tx, *lead)
	return err
}

// leadCompanyMatch is the company claiming the lead's domain, else the one its name names.
// The offer and the link both ask it, so the link confirms what the offer showed.
func leadCompanyMatch(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead) (*crmcontracts.LeadCompanyMatch, error) {
	match, err := domainCompanyMatch(ctx, tx, leadDomain(lead.CandidateCompanyKey))
	if err != nil || match != nil {
		return match, err
	}
	return uniqueCompanyMatch(ctx, tx, deref(lead.CompanyName))
}

// leadDomain is the registrable domain a lead's candidate_company_key holds,
// or "" when the key is a slug of the company name.
func leadDomain(key *string) string {
	if key == nil {
		return ""
	}
	domain, err := values.ParseDomain(*key)
	if err != nil {
		return ""
	}
	return domain.String()
}

// domainCompanyMatch is the live company the reader can open that claims domain, never the
// installation's own. uq_company_domain lets one company at most hold a live domain.
func domainCompanyMatch(ctx context.Context, tx pgx.Tx, domain string) (*crmcontracts.LeadCompanyMatch, error) {
	if domain == "" {
		return nil, nil //nolint:nilnil // a lead with no domain names no company by one
	}
	owners, err := exactCompanyByDomain(ctx, tx, []string{domain}, nil)
	if err != nil || len(owners) == 0 {
		return nil, err
	}
	matches, err := liveVisibleCompanies(ctx, tx, `id = $1 AND NOT is_anchor`, owners[0])
	if err != nil || len(matches) == 0 {
		return nil, err
	}
	matches[0].MatchedBy = crmcontracts.LeadCompanyMatchMatchedByDomain
	return &matches[0], nil
}

// uniqueCompanyMatch is the one live company the reader can open whose folded
// name equals companyName, or nil when none or more than one does.
func uniqueCompanyMatch(ctx context.Context, tx pgx.Tx, companyName string) (*crmcontracts.LeadCompanyMatch, error) {
	if strings.TrimSpace(companyName) == "" {
		return nil, nil //nolint:nilnil // no name means no match to offer, not a failure
	}
	matches, err := liveVisibleCompanies(ctx, tx,
		exactNameKeySQL("display_name")+` = `+exactNameKeySQL("$1"), companyName)
	if err != nil || len(matches) != 1 {
		return nil, err
	}
	matches[0].MatchedBy = crmcontracts.LeadCompanyMatchMatchedByName
	return &matches[0], nil
}

// liveVisibleCompanies is up to two live companies the reader can open that
// meet where, whose one bind is operand. A reader without company read opens none.
func liveVisibleCompanies[T ids.CompanyID | string](ctx context.Context, tx pgx.Tx, where string, operand T,
) ([]crmcontracts.LeadCompanyMatch, error) {
	if !auth.ReadGranted(ctx, companyEntity) {
		return nil, nil
	}
	args := []any{operand}
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
		   AND `+where+` AND `+scope+`
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
	return matches, nil
}

// unlinkUnnamedCompany drops the link when p moves the lead's company name or candidate key
// so neither names the linked company. name and key are the lead's values before p.
func unlinkUnnamedCompany(ctx context.Context, tx pgx.Tx, id ids.LeadID, name, key *string, p *storekit.Patch) error {
	moved := p.Moved()
	renamed, nameMoved := moved[leadCompanyColumn]
	rekeyed, keyMoved := moved[candidateCompanyKey]
	if !nameMoved && !keyMoved {
		return nil
	}
	linked, err := leadLinkedCompany(ctx, tx, id)
	if err != nil || linked == nil {
		return err
	}
	if nameMoved {
		name = textOrNil(renamed)
	}
	if keyMoved {
		key = textOrNil(rekeyed)
	}
	stillNamed, err := namesCompany(ctx, tx, *linked, name, key)
	if err != nil {
		return err
	}
	if !stillNamed {
		p.Set(leadCompanyIDColumn, linked.UUID, nil)
	}
	return nil
}

// textOrNil is a patched text column's new value, nil when it was cleared.
//
//craft:ignore naked-any storekit.Patch.Moved hands every column's value back as any
func textOrNil(value any) *string {
	if text, ok := value.(string); ok {
		return &text
	}
	return nil
}

// namesCompany reports whether a lead's company name or candidate key names a company,
// compared as the match offer compares. A blank name and a key that is no domain name none.
func namesCompany(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, name, key *string) (bool, error) {
	var named bool
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(`+exactNameKeySQL("display_name")+` = `+exactNameKeySQL("$2::text")+`, false)
		       OR EXISTS (SELECT 1 FROM company_domain claim
		                   WHERE claim.company_id = company.id AND claim.archived_at IS NULL AND claim.domain = $3)
		  FROM company WHERE id = $1`, companyID, name, leadDomain(key)).Scan(&named); err != nil {
		return false, fmt.Errorf("compare a lead's company name and domain with its linked company: %w", err)
	}
	return named, nil
}

// withholdUnreadableCompanies blanks company_id and company_facts on every lead whose
// company the caller cannot open, as withholdUnreadableSourceContacts does for the contact.
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
			leads[i].CompanyFacts = nil
		}
	}
	return nil
}
