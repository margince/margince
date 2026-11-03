// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	previewDeals        = "deals"
	previewCompanies    = "companies"
	previewContacts     = "contacts"
	previewCompanyLabel = "Company"
)

func (s Server) recordPreview(ctx context.Context, kind string, id ids.UUID) (recordPreview, error) {
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
	if deal.CompanyId != nil {
		company, err := s.contactsStore.GetCompany(ctx, ids.From[ids.CompanyKind](ids.UUID(*deal.CompanyId)), storekit.LiveOnly)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return recordPreview{}, err
		}
		if err == nil && company.MergedIntoId == nil {
			details = append(details, company.DisplayName)
		}
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
	if company.MergedIntoId != nil {
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
	if contact.MergedIntoId != nil {
		return recordPreview{}, apperrors.ErrNotFound
	}
	var details []string
	if contact.Title != nil {
		details = append(details, *contact.Title)
	}
	if contact.Employer != nil {
		details = append(details, contact.Employer.CompanyName)
	}
	return recordCard(previewContacts, id, contact.FullName, "Contact", details...), nil
}

func recordCard(kind string, id ids.UUID, name, label string, details ...string) recordPreview {
	if strings.TrimSpace(name) == "" {
		name = label
	}
	summary := strings.Join(slices.DeleteFunc(details, func(value string) bool { return strings.TrimSpace(value) == "" }), " · ")
	if summary == "" {
		summary = label + " in Margince"
	}
	return recordPreview{Title: "Margince · " + label + ": " + name, Summary: summary, Href: "/#/" + kind + "/" + id.String()}
}
