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

// hitTypeCompany is the `type` a company hit carries on the wire, and the same
// word the company branch declares in searchBranches.
const hitTypeCompany = "company"

// PartnerMarker answers which of these companies carry a live partner
// programme, for THIS caller, inside the caller's own transaction and for the
// whole page at once. A company missing from the returned map carries none
// the caller may see.
//
// It is the contacts store's own reader, injected by compose because a module
// never imports a sibling. Being a partner is a PROPERTY of a company and never
// a thing to look for, which is why it rides the company hit rather than
// joining searchBranches as a type of its own.
type PartnerMarker func(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID) (map[ids.CompanyID]bool, error)

// markPartners fills IsPartner on the company hits of one page.
//
// A caller who may not read partner programmes leaves every marker nil and
// still gets their search. Null on the wire means NOT KNOWN, which is the only
// honest answer for a seat that cannot see the programmes — false would tell
// them these accounts are not partners, which is a claim nobody made.
//
// Any other failure fails the search rather than answering with a silent gap,
// for the reason countTagReach does: the reader runs under the caller's own
// grant, so an error that is not a refusal is that read breaking, and a page
// that quietly dropped the markers would show a searcher partner accounts
// rendered as ordinary ones with no way to tell.
func (s *Store) markPartners(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	if s.partnerMarks == nil {
		return nil
	}
	var companyIDs []ids.CompanyID
	for i := range hits {
		if hits[i].Type == hitTypeCompany {
			companyIDs = append(companyIDs, ids.From[ids.CompanyKind](hits[i].ID))
		}
	}
	if len(companyIDs) == 0 {
		return nil
	}
	live, err := s.partnerMarks(ctx, tx, companyIDs)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: reading this page's partner programmes: %w", err)
	}
	for i := range hits {
		if hits[i].Type != hitTypeCompany {
			continue
		}
		isPartner := live[ids.From[ids.CompanyKind](hits[i].ID)]
		hits[i].IsPartner = &isPartner
	}
	return nil
}
