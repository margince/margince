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
	COALESCE(admission_reason, ` + domainPendingReason + `, ''),
	CASE WHEN pending_reason IS NOT NULL THEN pending_reason
	     ELSE COALESCE(admission_source, '') END,
	COALESCE(admission_at, updated_at),
	company_id`

// domainPendingReason turns a withholding reason into the sentence a reader
// gets told, for the two reasons the machine has for leaving a question open.
//
// Its own fragment because two surfaces render it: the operator's list above,
// which shows decisions and open questions together, and the owner's own
// backlog in domainquestionlist.go. A second spelling would be free to describe
// one withholding two ways, and the reader meeting both would have no way to
// tell which was the real ground.
//
// It answers the empty string for a row carrying no reason, which is every
// decided domain — the caller above COALESCEs that away behind the decision's
// own reason.
const domainPendingReason = `
	CASE pending_reason
	  WHEN '` + PendingUnevidenced + `'
	    THEN 'Nothing on the site named a company, and the sender''s name did not explain the domain.'
	  WHEN '` + PendingStaleEvidence + `'
	    THEN 'The newest mail from this domain is too old to trust today''s site as evidence about it.'
	  ELSE '' END`

// domainStandingWhere selects the rows this list is the right surface for: every
// decision, and the open questions nobody is in a position to answer.
//
// A bare `status = 'pending'` would sweep in every domain merely awaiting its
// turn in the crawl queue, which nobody needs to see — the sweep will get to it.
// What belongs here is the row whose retry cursor was CLEARED, which is what
// both withholding reasons do: nothing will ask again on its own.
//
// An open question owned by a LIVE colleague is deliberately absent. It reaches
// them in their own queue (domainquestionlist.go), with the two verbs that
// answer it — so carrying it here as well would put one question on two surfaces
// and let an operator answer for somebody whose mail they cannot read. That is
// the line this clause draws: between "somebody can still answer it" and "nobody
// can", never between readers.
//
// A question nobody can answer has to land here, because nothing else will show
// it. Two states reach that, and the second is why this asks about the owner's
// account rather than only about the column:
//
//   - owner_id IS NULL. The column is nullable, and the foreign key clears it
//     when the owner's app_user row is deleted (ON DELETE SET NULL).
//   - The owner is no longer live. Deactivation keeps the row — it writes
//     status='deactivated' and revokes every session — so owner_id still points
//     at somebody, and that somebody can no longer sign in to answer. The
//     per-owner read matches `owner_id = $1` for the CALLING human alone, so
//     without this arm the question is served to nobody: not to the departed
//     owner, who cannot authenticate, and not to any colleague.
//
// Either way the retry cursor is already cleared, so no sweep will re-ask and
// the row would be a company held out of the CRM with no surface admitting it
// exists.
//
// The liveness test is spelled here rather than shared with identity's
// LiveMemberSQL, which is the canonical copy: contacts may not import a sibling
// module. The two must agree, and identity/livemember_test.go asserts that
// predicate literally — so a change there that this misses shows up as a
// deactivated owner's questions reappearing on the operator's list.
//
// Note this admits an undecided row that also carries an admission: re-withholding
// an admitted domain leaves both columns set, and such a row belongs on a list of
// decisions whoever owns it.
const domainStandingWhere = `admission IS NOT NULL
	OR (pending_reason IS NOT NULL AND NOT EXISTS (
	      SELECT 1 FROM app_user u
	       WHERE u.id = company_domain_disposition.owner_id
	         AND u.status = 'active' AND u.archived_at IS NULL))`

// ListDomainAdmissions returns every domain that carries a decision, plus the
// open questions nobody owns, most recently moved first.
//
// Read-gated rather than write-gated: every role may SEE why a company is
// missing, while only admin/ops may change it. An operator who cannot find out
// that a domain was refused — or that nothing ever decided about it — has no way
// to know the CRM is not simply empty.
//
// It is NOT the whole backlog of open questions, and domainStandingWhere says
// why: a question a live colleague owns belongs to that colleague's queue, which
// is the only surface offering the two verbs that answer it. What lands here is
// the residue — the questions with nobody left to ask.
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
