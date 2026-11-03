// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A lead a partner referred: the link to that partner, and the acceptance a
// partner manager gives it. The first accepted lead activates the partner.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const leadPartnerField = "partner_company_id"

// PartnerLeadAcceptedError refuses a second acceptance. It also refuses a new
// partner on an accepted lead, because the stamp names the partner it was for.
type PartnerLeadAcceptedError struct{ code string }

func (e *PartnerLeadAcceptedError) Error() string {
	return "the lead was already accepted for its partner"
}

func (e *PartnerLeadAcceptedError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault names the contract's code.
func (e *PartnerLeadAcceptedError) MessageFault() (code, message string) {
	return e.code, e.Error()
}

// setLeadPartner folds a new or cleared referring partner onto the patch. The
// partner must be a live programme the caller can read, and the link is fixed
// once the lead was accepted for it.
func setLeadPartner(ctx context.Context, tx pgx.Tx, p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	cleared := slices.Contains(in.Clear, leadPartnerField)
	if in.PartnerCompanyID == nil && !cleared {
		return nil
	}
	stored, err := readStoredLeadPartner(ctx, tx, ids.From[ids.LeadKind](ids.UUID(current.Id)))
	if err != nil {
		return err
	}
	if stored.acceptedAt != nil {
		return &PartnerLeadAcceptedError{code: "partner_already_accepted"}
	}
	if in.PartnerCompanyID == nil {
		return nil
	}
	partner := *in.PartnerCompanyID
	if err := ensureLeadPartner(ctx, tx, partner); err != nil {
		return err
	}
	p.Set(leadPartnerField, stored.partner, partner.UUID)
	return nil
}

// storedLeadPartner is the lead's partner link as the row holds it.
type storedLeadPartner struct {
	partner    *ids.UUID
	acceptedAt *time.Time
}

// readStoredLeadPartner reads the partner link from the row, not from a read
// of the lead. A read blanks a partner the caller cannot open, and with it the
// stamp, while a decision about the link must see what is stored.
func readStoredLeadPartner(ctx context.Context, tx pgx.Tx, id ids.LeadID) (storedLeadPartner, error) {
	var out storedLeadPartner
	err := tx.QueryRow(ctx, `SELECT partner_company_id, partner_accepted_at FROM lead WHERE id = $1`, id).
		Scan(&out.partner, &out.acceptedAt)
	if err != nil {
		return storedLeadPartner{}, fmt.Errorf("read the lead's partner: %w", err)
	}
	return out, nil
}

// ensureLeadPartner refuses a seat without partner read, a referring partner
// the caller cannot read, or a company with no live partner programme. Create
// and update both name the partner through it, so neither tells a seat which
// companies are partners.
func ensureLeadPartner(ctx context.Context, tx pgx.Tx, partner ids.CompanyID) error {
	if err := auth.Require(ctx, partnerMaskObject, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureReferenceTarget(ctx, tx, leadPartnerField, companyEntity, partner.UUID); err != nil {
		return err
	}
	return EnsureCompanyIsPartner(ctx, tx, partner)
}

// LeadPartnerFilter narrows the lead list to the leads a partner referred.
type LeadPartnerFilter struct {
	// CompanyID keeps the leads this partner company referred.
	CompanyID *ids.CompanyID
	// Accepted keeps the referred leads a partner manager accepted (true), or
	// the ones still waiting for acceptance (false).
	Accepted *bool
}

// clauses renders the filter over the lead row. It reads partner and company
// data, so it takes both grants. Both arms read the partner through the
// caller's company scope. The lead's own read blanks a partner the caller
// cannot open, so no filter may match on one.
func (f LeadPartnerFilter) clauses(ctx context.Context, arg func(any) int) ([]string, error) {
	if f.CompanyID == nil && f.Accepted == nil {
		return nil, nil
	}
	if err := auth.Require(ctx, partnerMaskObject, principal.ActionRead); err != nil {
		return nil, err
	}
	partner, err := storekit.LinkedCompanyColumn(ctx, "lead."+leadPartnerField, "id", arg)
	if err != nil {
		return nil, err
	}
	var where []string
	if f.CompanyID != nil {
		where = append(where, storekit.SQLf("%s = $%d", partner, arg(*f.CompanyID)))
	}
	if f.Accepted != nil {
		stamp := "lead.partner_accepted_at IS NULL"
		if *f.Accepted {
			stamp = "lead.partner_accepted_at IS NOT NULL"
		}
		where = append(where, partner+" IS NOT NULL", stamp)
	}
	return where, nil
}

// withholdUnreadablePartners blanks the referring partner, and the acceptance
// stamp for it, on every lead whose partner company the caller cannot open.
func withholdUnreadablePartners(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	var partnerIDs []ids.UUID
	for _, l := range leads {
		if l.PartnerCompanyId != nil {
			partnerIDs = append(partnerIDs, ids.UUID(*l.PartnerCompanyId))
		}
	}
	if len(partnerIDs) == 0 {
		return nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, companyEntity, partnerIDs)
	if err != nil {
		return err
	}
	for i := range leads {
		if leads[i].PartnerCompanyId != nil && !visible[ids.UUID(*leads[i].PartnerCompanyId)] {
			// The stamp names the partner it was for, so it goes with the partner.
			leads[i].PartnerCompanyId = nil
			leads[i].PartnerAcceptedAt = nil
			leads[i].PartnerAcceptedBy = nil
		}
	}
	return nil
}

// AcceptPartnerLead records that a partner manager accepted the lead for the
// partner that referred it, and activates that partner when it is not yet. It
// takes the partner update grant and write access to the lead and to the
// partner company.
func (s *Store) AcceptPartnerLead(ctx context.Context, id ids.LeadID) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, partnerMaskObject, principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.Lead{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		lock, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived)
		if err != nil {
			return err
		}
		current, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		if err != nil {
			return err
		}
		// The stored partner, not the read's: a caller who cannot see the
		// partner must hear 404, not "no partner".
		stored, err := readStoredLeadPartner(ctx, tx, id)
		if err != nil {
			return err
		}
		partner, err := acceptablePartnerLead(ctx, tx, current, stored)
		if err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "company", partner.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		if err := EnsureCompanyIsPartner(ctx, tx, partner); err != nil {
			return err
		}
		if err := stampPartnerAcceptance(ctx, tx, lock, current, by); err != nil {
			return err
		}
		lead := id.UUID
		if _, err := activatePartnerTx(ctx, tx, partner,
			partnerActivationEvidence{basis: activationLeadAccepted, by: by, leadID: &lead}); err != nil {
			return err
		}
		out, err = readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		return err
	})
	return out, err
}

// acceptablePartnerLead answers the partner a lead may be accepted for, or
// why not. The lead may name no partner, have left the open ladder, or carry a
// stamp already. The caller may also be unable to write the partner company.
func acceptablePartnerLead(ctx context.Context, tx pgx.Tx, current crmcontracts.Lead, stored storedLeadPartner) (ids.CompanyID, error) {
	switch {
	case stored.acceptedAt != nil:
		return ids.CompanyID{}, &PartnerLeadAcceptedError{code: "already_accepted"}
	case current.MergedIntoId != nil:
		return ids.CompanyID{}, httperr.Validation(leadPartnerField, "lead_merged",
			"this lead was merged into another; accept the surviving lead instead")
	case current.Status == crmcontracts.LeadStatusDisqualified:
		return ids.CompanyID{}, httperr.Validation(leadPartnerField, "lead_disqualified",
			"a disqualified lead cannot be accepted for a partner; reopen it first")
	case current.ArchivedAt != nil:
		return ids.CompanyID{}, httperr.Validation(leadPartnerField, "lead_archived",
			"an archived lead cannot be accepted for a partner; restore it first")
	case stored.partner == nil:
		return ids.CompanyID{}, httperr.Validation(leadPartnerField, "no_partner",
			"name the partner that referred this lead before accepting it")
	}
	partner := ids.From[ids.CompanyKind](*stored.partner)
	// Acceptance writes the partner's activation, so it takes the write gate
	// every partner writer takes: an unseen partner is 404, a read share 403.
	if err := auth.EnsureWritable(ctx, tx, "company", partner.UUID); err != nil {
		return ids.CompanyID{}, err
	}
	return partner, nil
}

// stampPartnerAcceptance writes who accepted the lead and when, with its audit
// row and lead.updated.
func stampPartnerAcceptance(ctx context.Context, tx pgx.Tx, lock storekit.RowLock, current crmcontracts.Lead, by string) error {
	p := storekit.NewPatch()
	now := leadSLAClock().UTC()
	p.Set("partner_accepted_at", current.PartnerAcceptedAt, now)
	p.Set("partner_accepted_by", current.PartnerAcceptedBy, by)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("stamp the partner acceptance: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "lead", ids.UUID(current.Id), p.Before(), p.After())
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, ids.UUID(current.Id), crmcontracts.PublicEventLeadUpdated{ChangedFields: p.After()})
}
