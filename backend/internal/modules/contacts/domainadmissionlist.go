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
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// BlockedDomain is one domain's standing admission decision, as the admin list
// shows it.
type BlockedDomain struct {
	// ID is the disposition row, which the audit trail names. Not on the wire:
	// the domain is what an operator identifies a decision by.
	ID         ids.UUID
	Domain     string
	Admission  string
	Reason     string
	ReasonCode string
	SenderKind string
	Source     string
	DecidedAt  time.Time
	// CompanyID and CompanyName are set together by withVisibleCompanyTx, and
	// only for a company the caller may read.
	CompanyID   *ids.CompanyID
	CompanyName *string
}

// ToContractBlockedDomain is the wire shape of one admission decision.
//
// It lives beside the type rather than in a transport because two transports
// serve it now — the blocked-domain list and the company rejection — and a
// second spelling would be free to disagree about which fields reach a reader.
func ToContractBlockedDomain(e BlockedDomain) crmcontracts.BlockedDomain {
	out := crmcontracts.BlockedDomain{
		Domain:      e.Domain,
		Admission:   crmcontracts.BlockedDomainAdmission(e.Admission),
		Reason:      e.Reason,
		Source:      crmcontracts.BlockedDomainSource(e.Source),
		DecidedAt:   e.DecidedAt,
		CompanyName: e.CompanyName,
	}
	out.ReasonCode, out.Note, out.SenderKind = reasonCodeOf(e)
	if e.CompanyID != nil {
		id := openapi_types.UUID(e.CompanyID.UUID)
		out.CompanyId = &id
	}
	return out
}

// standingCompany is the company a disposition row points at, as read beside
// it and before anybody decided whether the caller may see it.
type standingCompany struct {
	id   *ids.UUID
	name *string
}

// withVisibleCompanyTx withholds both fields for a company the caller may not
// read: a mail-captured company stays owner-private even under row_scope=all.
func withVisibleCompanyTx(ctx context.Context, tx pgx.Tx, d *BlockedDomain, company standingCompany) error {
	if company.id == nil || company.name == nil {
		return nil
	}
	visible, err := auth.VisibleTo(ctx, tx, entityCompany, *company.id)
	if err != nil || !visible {
		return err
	}
	typed := ids.From[ids.CompanyKind](*company.id)
	d.CompanyID, d.CompanyName = &typed, company.name
	return nil
}

// domainStandingFrom names a disposition row `d` and the company it points at
// `c`, which every read of a domain's standing selects from.
const domainStandingFrom = `company_domain_disposition d LEFT JOIN company c ON c.id = d.company_id`

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
	CASE WHEN d.pending_reason IS NOT NULL THEN '` + DomainUndecided + `'
	     ELSE COALESCE(d.admission, '') END,
	COALESCE(d.admission_reason, ` + domainPendingReason + `, ''),
	CASE WHEN d.pending_reason IS NOT NULL THEN d.pending_reason
	     ELSE COALESCE(d.admission_source, '') END,
	COALESCE(d.admission_at, d.updated_at),
	d.company_id, c.display_name, ` + domainReasonCodeColumns

// domainPendingReason turns a withholding reason into the sentence a reader
// gets told, for each reason the machine has for leaving a question open.
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
	  WHEN '` + PendingNearDuplicate + `'
	    THEN 'An existing company carries this name. Confirm whether this domain belongs to it.'
	  WHEN '` + PendingForwardedIdentity + `'
	    THEN 'This domain forwards to a different site. Confirm its identity before linking a company.'
	  WHEN '` + PendingVendorVerdict + `'
	    THEN 'The site reads as a mail or hosting vendor. Create the company anyway?'
	  WHEN '` + PendingReservedDomain + `'
	    THEN 'Reserved domain: automatic company identification is paused.'
	  ELSE '' END`

// domainStandingWhere selects the rows this list is the right surface for: every
// decision, and the open questions nobody is in a position to answer.
//
// A bare `status = 'pending'` would sweep in every domain merely awaiting its
// turn in the crawl queue, which nobody needs to see — the sweep will get to it.
// What belongs here is the row whose retry cursor was CLEARED, which is what
// every withholding reason does: nothing will ask again on its own.
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
// Note this admits an undecided row that also carries an admission: re-withholding
// an admitted domain leaves both columns set, and such a row belongs on a list of
// decisions whoever owns it.
//
// A vendor question always lands here too. Its personal card goes only to the
// decider vendorDeciderSQL names, so this list keeps the fact
// reachable when no seat owes it.
var domainStandingWhere = `d.admission IS NOT NULL OR d.pending_reason IN ('` + PendingReservedDomain + `', '` + PendingVendorVerdict + `')
	OR (d.pending_reason IS NOT NULL AND NOT EXISTS (
	      SELECT 1 FROM company_domain_question_reader r JOIN app_user u ON u.id = r.reader_id
	       WHERE r.domain = d.domain
	         AND ` + workforce.LiveMemberSQL("u") + `))`

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
			SELECT count(*) FROM company_domain_disposition d
			 WHERE `+domainStandingWhere).Scan(&total); err != nil {
			return fmt.Errorf("contacts: counting domain admissions: %w", err)
		}
		// Ordered on the same COALESCE the row is built from: an undecided row
		// has no admission_at, and ordering on that column alone would sort
		// every open question into one heap at the end regardless of when it
		// was last touched — burying the newest question under the oldest.
		rows, err := tx.Query(ctx, `
			SELECT d.id, d.domain, `+domainStanding+`
			  FROM `+domainStandingFrom+`
			 WHERE `+domainStandingWhere+`
			 ORDER BY COALESCE(d.admission_at, d.updated_at) DESC
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("contacts: listing domain admissions: %w", err)
		}
		// Collected BEFORE any per-row visibility query: the rows cursor holds
		// the connection, and a second query on the same transaction while it
		// is open answers "conn busy".
		var companies []standingCompany
		for rows.Next() {
			var d BlockedDomain
			var company standingCompany
			if err := rows.Scan(&d.ID, &d.Domain, &d.Admission, &d.Reason, &d.Source, &d.DecidedAt,
				&company.id, &company.name, &d.ReasonCode, &d.SenderKind); err != nil {
				rows.Close()
				return fmt.Errorf("contacts: reading a domain admission: %w", err)
			}
			out = append(out, d)
			companies = append(companies, company)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("contacts: listing domain admissions: %w", err)
		}
		for i, company := range companies {
			if err := withVisibleCompanyTx(ctx, tx, &out[i], company); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
