// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	previewDeals        = "deals"
	previewCompanies    = "companies"
	previewContacts     = "contacts"
	previewCompanyLabel = "Company"
)

func (s Server) recordPreview(ctx context.Context, kind string, id ids.UUID) (recordPreview, error) {
	ctx = s.coreFeatures.Context(ctx)
	if !s.coreFeatures.AreaEnabled(kind) {
		return recordPreview{}, apperrors.ErrNotFound
	}
	switch kind {
	case previewDeals:
		return s.dealRecordPreview(ctx, id)
	case previewCompanies:
		return s.companyRecordPreview(ctx, id)
	case previewContacts:
		return s.contactRecordPreview(ctx, id)
	default:
		return recordPreview{}, apperrors.ErrNotFound
	}
}

func (s Server) dealRecordPreview(ctx context.Context, id ids.UUID) (recordPreview, error) {
	deal, err := s.dealsStore.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
	if err != nil {
		return recordPreview{}, err
	}
	var details []string
	if deal.CompanyId != nil && !cardWithholds(ctx, fieldmask.DealCompanyID) {
		name, err := s.cardCompanyName(ctx, ids.UUID(*deal.CompanyId))
		if err != nil {
			return recordPreview{}, err
		}
		details = append(details, name)
	}
	if deal.StageId != nil {
		stage, err := s.dealsStore.GetStage(ctx, ids.From[ids.StageKind](ids.UUID(*deal.StageId)))
		if err != nil {
			return recordPreview{}, err
		}
		details = append(details, stage.Name)
	}
	return recordCard(previewDeals, id, deal.Name, "Deal", details...), nil
}

func (s Server) companyRecordPreview(ctx context.Context, id ids.UUID) (recordPreview, error) {
	company, err := s.contactsStore.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.LiveOnly)
	if err != nil {
		return recordPreview{}, err
	}
	if company.MergedIntoId != nil || ownerPrivate(company.Visibility, crmcontracts.CompanyVisibilityOwner) {
		return recordPreview{}, apperrors.ErrNotFound
	}
	var domainName string
	if company.Domains != nil {
		for _, domain := range *company.Domains {
			if domain.IsPrimary {
				domainName = domain.Domain
				break
			}
		}
	}
	return recordCard(previewCompanies, id, company.DisplayName, previewCompanyLabel, domainName), nil
}

func (s Server) contactRecordPreview(ctx context.Context, id ids.UUID) (recordPreview, error) {
	contact, err := s.contactsStore.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
	if err != nil {
		return recordPreview{}, err
	}
	if contact.MergedIntoId != nil || ownerPrivate(contact.Visibility, crmcontracts.ContactVisibilityOwner) {
		return recordPreview{}, apperrors.ErrNotFound
	}
	var details []string
	if contact.Title != nil {
		details = append(details, *contact.Title)
	}
	if contact.Employer != nil {
		name, err := s.cardCompanyName(ctx, ids.UUID(contact.Employer.CompanyId))
		if err != nil {
			return recordPreview{}, err
		}
		details = append(details, name)
	}
	return recordCard(previewContacts, id, contact.FullName, "Contact", details...), nil
}

// cardCompanyName is the company name a card may print beside another record.
// A company that is gone, merged away, or private to its mailbox owner has
// none. The card is read as the system, which capture privacy does not stop.
func (s Server) cardCompanyName(ctx context.Context, id ids.UUID) (string, error) {
	if !auth.ReadGranted(ctx, "company") {
		return "", nil
	}
	company, err := s.contactsStore.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if company.MergedIntoId != nil || ownerPrivate(company.Visibility, crmcontracts.CompanyVisibilityOwner) {
		return "", nil
	}
	return company.DisplayName, nil
}

type cardWithheldKey struct{}

// withCardWithheld carries the deal fields some role withholds to the card,
// which has no reader whose own masks it could ask.
func withCardWithheld(ctx context.Context, withheld []string) context.Context {
	return context.WithValue(ctx, cardWithheldKey{}, withheld)
}

// cardWithholds reports whether the card leaves the deal field out. A card
// reached without the installation's masks leaves every field out.
func cardWithholds(ctx context.Context, field string) bool {
	withheld, known := ctx.Value(cardWithheldKey{}).([]string)
	return !known || slices.Contains(withheld, field)
}

// ownerPrivate reports a row capture privacy keeps for its mailbox owner, which
// a card anyone may open never names.
func ownerPrivate[V comparable](visibility *V, owner V) bool {
	return visibility != nil && *visibility == owner
}

func recordCard(kind string, id ids.UUID, name, label string, details ...string) recordPreview {
	summary := strings.Join(slices.DeleteFunc(details, func(value string) bool { return strings.TrimSpace(value) == "" }), " · ")
	preview := recordPreview{kind: kind, name: strings.TrimSpace(name), genericSummary: summary == "", Summary: summary, Href: "/#/" + kind + "/" + id.String()}
	card := recordPreviewCard(preview)
	preview.Title, preview.Summary = card.Title, card.Summary
	return preview
}
