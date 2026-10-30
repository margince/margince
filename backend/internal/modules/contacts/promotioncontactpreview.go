// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func promotionContactSpec(lead crmcontracts.Lead, name, by string) ContactSpec {
	leadID := ids.UUID(lead.Id)
	var leadEmails []ContactEmailInput
	if lead.Email != nil {
		leadEmails = []ContactEmailInput{{
			Email: string(*lead.Email), EmailType: emailTypeWork, IsPrimary: true, Position: 1,
		}}
	}
	var leadPhones []ContactPhoneInput
	if lead.Phone != nil && *lead.Phone != "" {
		leadPhones = []ContactPhoneInput{{
			Phone: *lead.Phone, PhoneType: phoneTypeWork, IsPrimary: true, Position: 1,
		}}
	}
	return ContactSpec{
		FullName:            name,
		Employer:            EmployerChoice{CompanyName: lead.CompanyName, CompanyID: idArg[ids.CompanyKind](lead.CompanyId), Role: lead.Title, Source: lead.Source},
		Acquisition:         Acquisition{Kind: AcquiredUnknownLegacy},
		Title:               lead.Title,
		OwnerID:             ownerFromUUID(uuidPtrToIDs(lead.OwnerId)),
		ConvertedFromLeadID: &leadID,
		converting:          []ids.LeadID{ids.From[ids.LeadKind](leadID)},
		Emails:              leadEmails,
		Phones:              leadPhones,
		Source:              lead.Source,
		CapturedBy:          by,
	}
}

// promotionAcquisition preserves what the lead's address proves about its
// origin. The lead is still live while this evidence is read.
func promotionAcquisition(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead) (Acquisition, error) {
	if lead.Email == nil {
		return Acquisition{Kind: AcquiredUnknownLegacy}, nil
	}
	email := string(*lead.Email)
	kind, err := acquiredFromCaptureTx(ctx, tx, false, email)
	if err != nil {
		return Acquisition{}, err
	}
	return acquisitionFromCapture(ctx, tx, kind, email, ids.ActivityID{}), nil
}

func promotionContactChange(lead crmcontracts.Lead, current crmcontracts.Contact) (crmcontracts.Contact, *storekit.Patch) {
	p := storekit.NewPatch()
	if current.ConvertedFromLeadId == nil {
		p.Set("converted_from_lead_id", nil, ids.UUID(lead.Id))
		current.ConvertedFromLeadId = new(lead.Id)
	}
	if current.Title == nil && lead.Title != nil {
		p.Set(fieldTitle, nil, *lead.Title)
		current.Title = lead.Title
	}
	return current, p
}

func leadPhonesToCarry(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, contactID ids.ContactID) ([]ContactPhoneInput, error) {
	if lead.Phone == nil || *lead.Phone == "" {
		return []ContactPhoneInput{}, nil
	}
	held, err := heldContactPhones(ctx, tx, contactID)
	if err != nil {
		return nil, err
	}
	return promotionPhones(lead, held), nil
}

func promotionPhones(lead crmcontracts.Lead, held map[string][]heldPhone) []ContactPhoneInput {
	if lead.Phone == nil || *lead.Phone == "" {
		return []ContactPhoneInput{}
	}
	if len(held[*lead.Phone]) > 0 {
		return []ContactPhoneInput{}
	}
	return []ContactPhoneInput{{Phone: *lead.Phone, PhoneType: phoneTypeWork, Position: len(held) + 1}}
}

// LeadContactChange binds contact changes to the lead and contact versions reviewed.
type LeadContactChange struct {
	EmployerPreview ContactEmployerPreview
	Employer        EmployerChoice
	LeadPromotionCarry
	LeadVersion    int64
	Create         *ContactSpec
	ContactID      ids.ContactID
	ContactVersion int64
	Patch          *storekit.Patch
	Phones         []ContactPhoneInput
	Deal           *QualifyDealInput
	ReviewContact  *ids.ContactID
	// CompanyIndustry is the industry the promotion writes to the linked company, or nil.
	CompanyIndustry *string
}

// PreviewLeadContactChange holds the writer's row locks while resolving the
// contact and its field changes; it does not perform the promotion.
func (s *Store) PreviewLeadContactChange(ctx context.Context, id ids.LeadID, in PromoteLeadInput) (LeadContactChange, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return LeadContactChange{}, err
	}
	if err := auth.Require(ctx, leadEntity, principal.ActionUpdate); err != nil {
		return LeadContactChange{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionCreate); err != nil {
		return LeadContactChange{}, err
	}
	if _, err := ParsePromoteTrigger(in.Trigger); err != nil {
		return LeadContactChange{}, err
	}
	if err := promotionFieldsVisible(ctx, leadEntity); err != nil {
		return LeadContactChange{}, err
	}
	var out LeadContactChange
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.previewLeadContactChangeTx(ctx, tx, id, in)
		return err
	})
	if err != nil {
		return LeadContactChange{}, err
	}
	return out, nil
}

func promotionFieldsVisible(ctx context.Context, object string) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	for _, field := range auth.MaskedFields(actor, object, true) {
		switch field {
		case contactNameColumn, fieldEmail, fieldPhone, contactPhonesField, fieldTitle, filterOwnerID, fieldSource, "converted_from_lead_id", "version", "company_name":
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
}

func previewPromotionContact(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, candidate ids.ContactID) (LeadContactChange, error) {
	if lead.Version == nil {
		return LeadContactChange{}, apperrors.ErrPermissionDenied
	}
	out := LeadContactChange{LeadVersion: int64(*lead.Version), ContactID: candidate}
	if candidate.IsZero() {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return LeadContactChange{}, err
		}
		out.Create = new(promotionContactSpec(lead, leadIdentityName(lead), by))
		return out, nil
	}
	current, err := readContact(ctx, tx, candidate, storekit.LiveOnly, nil)
	if err != nil {
		return LeadContactChange{}, err
	}
	if current.Version == nil {
		return LeadContactChange{}, apperrors.ErrPermissionDenied
	}
	out.ContactVersion = int64(*current.Version)
	_, out.Patch = promotionContactChange(lead, current)
	out.Phones, err = leadPhonesToCarry(ctx, tx, lead, candidate)
	return out, err
}

func promotionCandidateVisible(ctx context.Context, tx pgx.Tx, candidate ids.ContactID) error {
	if candidate.IsZero() {
		return nil
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, contactEntity, candidate.UUID); err != nil {
		return err
	}
	return promotionFieldsVisible(ctx, contactEntity)
}

func previewPromotionReview(ctx context.Context, tx pgx.Tx, match ContactResolution, out *LeadContactChange) error {
	if !match.Decision.queuesReview() {
		return nil
	}
	// A hidden match changes nothing the promoter sees.
	shown, err := firstVisibleReview(ctx, tx, match)
	if err != nil || shown == nil {
		return err
	}
	out.ReviewContact = &shown.ContactID
	return nil
}

func (s *Store) previewLeadContactChangeTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (LeadContactChange, error) {
	candidate, err := s.promoteCandidate(ctx, tx, id)
	if err != nil {
		return LeadContactChange{}, err
	}
	if err := promotionCandidateVisible(ctx, tx, candidate); err != nil {
		return LeadContactChange{}, err
	}
	if err := lockContactThenLead(ctx, tx, candidate, storekit.IncludeArchived, id, nil); err != nil {
		return LeadContactChange{}, err
	}
	lead, err := promotableLead(ctx, tx, id, in)
	if err != nil {
		return LeadContactChange{}, err
	}
	if err := refuseIfVersionMoved(leadEntity, lead.Version, writeOptions{atVersion: in.IfVersion}); err != nil {
		return LeadContactChange{}, err
	}
	match, err := s.previewTarget(ctx, tx, lead)
	if err != nil {
		return LeadContactChange{}, err
	}
	matched := match.Decision == DecisionExactCollision
	if (matched && match.ContactID != candidate) || (!matched && !candidate.IsZero()) {
		return LeadContactChange{}, apperrors.ErrConflict
	}
	out, err := previewPromotionContact(ctx, tx, lead, candidate)
	if err != nil {
		return LeadContactChange{}, err
	}
	if err := previewPromotionReview(ctx, tx, match, &out); err != nil {
		return LeadContactChange{}, err
	}
	if err := s.previewPromotionCarry(ctx, tx, id, candidate, &out.LeadPromotionCarry); err != nil {
		return LeadContactChange{}, err
	}
	spec := promotionContactSpec(lead, leadIdentityName(lead), "")
	out.Employer = spec.Employer
	out.EmployerPreview, err = s.previewContactEmployerTx(ctx, tx, spec.Employer, spec.Emails)
	if err != nil {
		return LeadContactChange{}, err
	}
	company, err := leadLinkedCompany(ctx, tx, id)
	if err != nil {
		return LeadContactChange{}, err
	}
	if out.CompanyIndustry, err = leadIndustryCarry(ctx, tx, company, lead.Industry); err != nil {
		return LeadContactChange{}, err
	}
	out.Deal, err = s.previewQualifiedDeal(ctx, tx, lead, in.Deal)
	return out, err
}
