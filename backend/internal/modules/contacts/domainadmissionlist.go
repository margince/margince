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

// domainStanding is where a domain's company question stands, as one row of the
// operator's list: the decision it carries, or — when nobody decided — the
// reason the machine left it open.
//
// The two states are one SELECT because they are one question. Undecided is
// keyed on pending_reason rather than on a missing admission: the table's
// pending_reason_shape constraint makes a reason imply status='pending', so a
// row carrying one is exactly a question nothing answered. A SETTLED domain
// also has no admission — settleDisposition never writes that column — and
// reading absence as "undecided" would call every answered domain an open
// question, offer a re-ask that silently matches no row, and hand back a
// company id the list is careful to withhold.
//
// Shared with the single-row read in domainopenquestion.go, so a re-ask answers
// in the same shape the operator was just looking at.
const domainStanding = `
	CASE WHEN pending_reason IS NOT NULL THEN '` + DomainUndecided + `'
	     ELSE COALESCE(admission, '') END,
	COALESCE(admission_reason,
	         CASE pending_reason
	           WHEN '` + PendingUnevidenced + `'
	             THEN 'Nothing on the site named a company, and the sender''s name did not explain the domain.'
	           WHEN '` + PendingStaleEvidence + `'
	             THEN 'The newest mail from this domain is too old to trust today''s site as evidence about it.'
	           ELSE '' END, ''),
	CASE WHEN pending_reason IS NOT NULL THEN pending_reason
	     ELSE COALESCE(admission_source, '') END,
	COALESCE(admission_at, updated_at),
	company_id`

// domainStandingWhere selects the rows that have something to say: a decision,
// or an open question somebody must answer.
//
// A bare `status = 'pending'` would sweep in every domain merely awaiting its
// turn in the crawl queue, which nobody needs to see — the sweep will get to it.
// What belongs here is the row whose retry cursor was CLEARED, which is what
// both withholding reasons do: nothing will ask again on its own.
const domainStandingWhere = `admission IS NOT NULL OR pending_reason IS NOT NULL`

// ListDomainAdmissions returns every domain whose company question has an answer
// or is waiting for one, most recently moved first.
//
// Read-gated rather than write-gated: every role may SEE why a company is
// missing, while only admin/ops may change it. An operator who cannot find out
// that a domain was refused — or that nothing ever decided about it — has no way
// to know the CRM is not simply empty.
func (s *Store) ListDomainAdmissions(ctx context.Context, limit int) ([]BlockedDomain, int, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return nil, 0, err
	}
	var out []BlockedDomain
	var total int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM company_domain_disposition
			 WHERE `+domainStandingWhere).Scan(&total); err != nil {
			return fmt.Errorf("contacts: counting domain admissions: %w", err)
		}
		// Ordered on the same COALESCE the row is built from: an undecided row
		// has no admission_at, and ordering on that column alone would sort
		// every open question into one heap at the end regardless of when it
		// was last touched — burying the newest question under the oldest.
		rows, err := tx.Query(ctx, `
			SELECT id, domain, `+domainStanding+`
			  FROM company_domain_disposition
			 WHERE `+domainStandingWhere+`
			 ORDER BY COALESCE(admission_at, updated_at) DESC
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
