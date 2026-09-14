// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The admin's READ of the admission decisions: what every role may see about
// why a company is missing. The decisions themselves are made in
// domainadmission.go, which is where the sticky rule and the write live.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BlockedDomain is one domain's standing admission decision, as the admin list
// shows it.
type BlockedDomain struct {
	// ID is the disposition row, which the audit trail names. Not on the wire:
	// the domain is what an operator identifies a decision by.
	ID        ids.UUID
	Domain    string
	Admission string
	Reason    string
	Source    string
	DecidedAt time.Time
	CompanyID *ids.CompanyID
}

// ToContractBlockedDomain is the wire shape of one admission decision.
//
// It lives beside the type rather than in a transport because two transports
// serve it now — the blocked-domain list and the company rejection — and a
// second spelling would be free to disagree about which fields reach a reader.
func ToContractBlockedDomain(e BlockedDomain) crmcontracts.BlockedDomain {
	out := crmcontracts.BlockedDomain{
		Domain:    e.Domain,
		Admission: crmcontracts.BlockedDomainAdmission(e.Admission),
		Reason:    e.Reason,
		Source:    crmcontracts.BlockedDomainSource(e.Source),
		DecidedAt: e.DecidedAt,
	}
	if e.CompanyID != nil {
		id := openapi_types.UUID(e.CompanyID.UUID)
		out.CompanyId = &id
	}
	return out
}

// ListDomainAdmissions returns every domain carrying a decision, newest first —
// the refusals the system made and the ones a human overrode.
//
// Read-gated rather than write-gated: every role may SEE why a company is
// missing, while only admin/ops may change it. An operator who cannot find out
// that a domain was refused has no way to know the CRM is not simply empty.
func (s *Store) ListDomainAdmissions(ctx context.Context, limit int) ([]BlockedDomain, int, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return nil, 0, err
	}
	var out []BlockedDomain
	var total int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM company_domain_disposition
			 WHERE admission IS NOT NULL`).Scan(&total); err != nil {
			return fmt.Errorf("contacts: counting domain admissions: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT id, domain, admission, COALESCE(admission_reason, ''),
			       COALESCE(admission_source, ''), admission_at, company_id
			  FROM company_domain_disposition
			 WHERE admission IS NOT NULL
			 ORDER BY admission_at DESC
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("contacts: listing domain admissions: %w", err)
		}
		// Collected BEFORE any per-row visibility query: the rows cursor holds
		// the connection, and a second query on the same transaction while it
		// is open answers "conn busy".
		var companyIDs []*ids.UUID
		for rows.Next() {
			var d BlockedDomain
			var companyID *ids.UUID
			if err := rows.Scan(&d.ID, &d.Domain, &d.Admission, &d.Reason, &d.Source, &d.DecidedAt, &companyID); err != nil {
				rows.Close()
				return fmt.Errorf("contacts: reading a domain admission: %w", err)
			}
			out = append(out, d)
			companyIDs = append(companyIDs, companyID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("contacts: listing domain admissions: %w", err)
		}
		for i, companyID := range companyIDs {
			if companyID == nil {
				continue
			}
			// The company id is withheld unless the caller could read that
			// company. A company captured from mail is owner-PRIVATE
			// until a human promotes it, and that privacy does not yield to
			// row_scope=all — so returning the id here would hand every
			// colleague a pointer to a record the record's own endpoint
			// correctly 404s. Same rule, and same VisibleTo check, as the
			// duplicate-domain refusal in company_domains.go.
			visible, verr := auth.VisibleTo(ctx, tx, entityCompany, *companyID)
			if verr != nil {
				return verr
			}
			if visible {
				typed := ids.From[ids.CompanyKind](*companyID)
				out[i].CompanyID = &typed
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
