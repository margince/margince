// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	fieldContactIDs      = "contact_ids"
	codeNotLinkCandidate = "not_a_link_candidate"
)

// LinkCompanyContacts writes a current employment edge from each named contact
// to the company, all or none. Every id must be a link candidate for this
// caller now; the first that is not refuses the whole batch by name.
func (s *Store) LinkCompanyContacts(ctx context.Context, companyID ids.CompanyID, contactIDs []ids.UUID) ([]relationshipRow, error) {
	if err := linkBatchShape(contactIDs); err != nil {
		return nil, err
	}
	for _, object := range []string{companyEntity, contactEntity, tableRelationship} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return nil, err
		}
	}
	if err := requireRelationshipCreate(ctx, contactEntity); err != nil {
		return nil, err
	}
	var out []relationshipRow
	err := s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		out = make([]relationshipRow, 0, len(contactIDs))
		if err := auth.EnsureVisibleLive(ctx, tx, companyEntity, companyID.UUID); err != nil {
			return err
		}
		candidates, err := s.linkCandidatesTx(ctx, tx, companyID, contactIDs, len(contactIDs))
		if err != nil {
			return err
		}
		if err := refuseNonCandidates(contactIDs, candidates); err != nil {
			return err
		}
		for _, id := range contactIDs {
			attached, err := s.attachEmployerTx(ctx, tx, ids.From[ids.ContactKind](id),
				EmployerChoice{CompanyID: &companyID, Source: linkEmployerSource})
			if err != nil {
				return err
			}
			out = append(out, attached.edge)
		}
		return nil
	})
	return out, err
}

func linkBatchShape(contactIDs []ids.UUID) error {
	if len(contactIDs) == 0 {
		return &RequiredFieldError{Field: fieldContactIDs}
	}
	if len(contactIDs) > linkCandidateCap {
		return httperr.Validation(fieldContactIDs, "too_many", fmt.Sprintf("link at most %d contacts at once", linkCandidateCap))
	}
	seen := make(map[ids.UUID]bool, len(contactIDs))
	for _, id := range contactIDs {
		if seen[id] {
			return contactIDRefusal("duplicate", id, "this contact is named twice")
		}
		seen[id] = true
	}
	return nil
}

func refuseNonCandidates(contactIDs []ids.UUID, candidates []LinkCandidate) error {
	offered := make(map[ids.UUID]bool, len(candidates))
	for _, c := range candidates {
		offered[c.ContactID.UUID] = true
	}
	for _, id := range contactIDs {
		if !offered[id] {
			return contactIDRefusal(codeNotLinkCandidate, id,
				"this contact is not one you can link here: not on the company's domains, a workspace seat, not yours to change, or already employed there")
		}
	}
	return nil
}

// contactIDRefusal names the offending id in details, since one field carries
// the whole list.
func contactIDRefusal(code string, id ids.UUID, message string) error {
	refusal := httperr.Validation(fieldContactIDs, code, message)
	refusal.Details = map[string]any{"contact_id": id.String()}
	return refusal
}
