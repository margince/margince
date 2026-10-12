// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The company pass: a row about an account says which side wrote last, as the
// contact pass does for a row about a human. One read for every company the
// page names, after the page is cut, for the reason the contact pass gives.

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyTouch answers when each of a set of accounts last wrote to us and
// when we last wrote to it, under the caller's own grants, in a single call
// per page. An account the caller may not read is absent from the answer; a
// caller who may not read activity is refused with
// apperrors.ErrPermissionDenied.
type CompanyTouch interface {
	LastTouch(ctx context.Context, companyIDs []ids.UUID) (map[ids.UUID]TouchMoments, error)
}

// nameTheCompanies marks every row whose subject is a company with that
// company, and fills the two moments from one CompanyTouch read for them all.
func (s *Service) nameTheCompanies(ctx context.Context, rows []crmcontracts.WorklistItem) error {
	named := companiesOn(rows)
	if len(named) == 0 || s.companyTouch == nil {
		return nil
	}
	moments, err := s.companyTouch.LastTouch(ctx, named)
	// Refused is withheld, not failed: every row keeps its company and no row
	// claims a date.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("attention: reading when the accounts on the queue last wrote: %w", err)
	}
	for i := range rows {
		company := rows[i].Company
		if company == nil {
			continue
		}
		touch, known := moments[ids.UUID(company.Id)]
		if !known {
			continue
		}
		company.Touch = &crmcontracts.WorklistContactTouch{
			LastInboundAt:  touch.LastInbound,
			LastOutboundAt: touch.LastOutbound,
		}
	}
	return nil
}

// companiesOn sets Company on every row about a company and gathers those
// companies without repeats, in the order they were met.
func companiesOn(rows []crmcontracts.WorklistItem) []ids.UUID {
	var named []ids.UUID
	seen := map[ids.UUID]bool{}
	for i := range rows {
		subject := rows[i].Subject
		if subject == nil || subject.Type != subjectCompany {
			continue
		}
		rows[i].Company = &crmcontracts.WorklistCompanyFacts{Id: subject.Id}
		id := ids.UUID(subject.Id)
		if seen[id] {
			continue
		}
		seen[id] = true
		named = append(named, id)
	}
	return named
}
