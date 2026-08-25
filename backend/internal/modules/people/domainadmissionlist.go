// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The admin's READ of the admission decisions: what every role may see about
// why a company is missing. The decisions themselves are made in
// domainadmission.go, which is where the sticky rule and the write live.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// BlockedDomain is one domain's standing admission decision, as the admin list
// shows it.
type BlockedDomain struct {
	// ID is the disposition row, which the audit trail names. Not on the wire:
	// the domain is what an operator identifies a decision by.
	ID             ids.UUID
	Domain         string
	Admission      string
	Reason         string
	Source         string
	DecidedAt      time.Time
	OrganizationID *ids.OrganizationID
}

// ListDomainAdmissions returns every domain carrying a decision, newest first —
// the refusals the system made and the ones a human overrode.
//
// Read-gated rather than write-gated: every role may SEE why a company is
// missing, while only admin/ops may change it. An operator who cannot find out
// that a domain was refused has no way to know the CRM is not simply empty.
func (s *Store) ListDomainAdmissions(ctx context.Context, limit int) ([]BlockedDomain, int, error) {
	if err := auth.Require(ctx, entityOrganization, principal.ActionRead); err != nil {
		return nil, 0, err
	}
	var out []BlockedDomain
	var total int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM organization_domain_disposition
			 WHERE admission IS NOT NULL`).Scan(&total); err != nil {
			return fmt.Errorf("people: counting domain admissions: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT id, domain, admission, COALESCE(admission_reason, ''),
			       COALESCE(admission_source, ''), admission_at, organization_id
			  FROM organization_domain_disposition
			 WHERE admission IS NOT NULL
			 ORDER BY admission_at DESC
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("people: listing domain admissions: %w", err)
		}
		// Collected BEFORE any per-row visibility query: the rows cursor holds
		// the connection, and a second query on the same transaction while it
		// is open answers "conn busy".
		var orgIDs []*ids.UUID
		for rows.Next() {
			var d BlockedDomain
			var orgID *ids.UUID
			if err := rows.Scan(&d.ID, &d.Domain, &d.Admission, &d.Reason, &d.Source, &d.DecidedAt, &orgID); err != nil {
				rows.Close()
				return fmt.Errorf("people: reading a domain admission: %w", err)
			}
			out = append(out, d)
			orgIDs = append(orgIDs, orgID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("people: listing domain admissions: %w", err)
		}
		for i, orgID := range orgIDs {
			if orgID == nil {
				continue
			}
			// The company id is withheld unless the caller could read that
			// company. An organization captured from mail is owner-PRIVATE
			// until a human promotes it, and that privacy does not yield to
			// row_scope=all — so returning the id here would hand every
			// colleague a pointer to a record the record's own endpoint
			// correctly 404s. Same rule, and same VisibleTo check, as the
			// duplicate-domain refusal in organization_domains.go.
			visible, verr := auth.VisibleTo(ctx, tx, entityOrganization, *orgID)
			if verr != nil {
				return verr
			}
			if visible {
				typed := ids.From[ids.OrganizationKind](*orgID)
				out[i].OrganizationID = &typed
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
