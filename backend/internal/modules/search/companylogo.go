// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyLogoReader answers the logo URL of each of these companies that wears
// one, for THIS caller, inside the caller's own transaction and for the whole
// page at once. A company missing from the map wears none the caller may see.
//
// It is the contacts store's own reader, injected by compose because a module
// never imports a sibling, and so the URL comes from contacts.LogoURL, the
// spelling the company record itself carries.
type CompanyLogoReader func(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID) (map[ids.CompanyID]string, error)

// attachCompanyLogos fills LogoURL on the company hits of one page, under
// markPartners' policy: a refusal leaves every logo absent and the search
// answers, while any other failure fails the search rather than quietly
// drawing every account as initials.
func (s *Store) attachCompanyLogos(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	if s.companyLogos == nil {
		return nil
	}
	companyIDs := companyHitIDs(hits)
	if len(companyIDs) == 0 {
		return nil
	}
	logos, err := s.companyLogos(ctx, tx, companyIDs)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: reading this page's company logos: %w", err)
	}
	for i := range hits {
		if hits[i].Type != hitTypeCompany {
			continue
		}
		if logoURL, ok := logos[ids.From[ids.CompanyKind](hits[i].ID)]; ok {
			hits[i].LogoURL = &logoURL
		}
	}
	return nil
}
