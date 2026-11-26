// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Noticing that a contact moved. Capture never reassigns an employer
// (employmentedge.go), so this asks the contact's owner instead.
//
// Anyone can send mail. So the evidence is only mail the contact sent from an
// address on their record, vouched for by its domain. The owner may read it.
// It names a company the owner may see, and is newer than all that places the
// contact at the employer we hold.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// EmployerChangeProposer puts employer changes to the contacts' owners, in the
// transaction that captured the evidence. Compose binds the approvals inbox;
// nil asks nobody.
type EmployerChangeProposer interface {
	ProposeEmployerChangesTx(ctx context.Context, tx pgx.Tx, changes []approvalsubject.EmployerChange) error
}

// WithEmployerChangeProposer wires the inbox an employer change is asked in.
func (s *Store) WithEmployerChangeProposer(proposer EmployerChangeProposer) *Store {
	s.employerChanges = proposer
	return s
}

// WithEmployerChangeProposer is Store.WithEmployerChangeProposer for the store
// behind these handlers, where a member settles a domain question.
func (h Handlers) WithEmployerChangeProposer(proposer EmployerChangeProposer) Handlers {
	h.store = h.store.WithEmployerChangeProposer(proposer)
	return h
}

// employerSignal is one captured mail that places a contact at a company:
// through its mail domain, or through the company its signature names. domain
// is folded, and empty for a signature.
type employerSignal struct {
	contact   ids.ContactID
	company   ids.CompanyID
	activity  ids.UUID
	domain    string
	signature string
}

// domainEmployerChangeCap bounds the questions one settled domain raises in
// its request. A contact past it is asked when they next write.
const domainEmployerChangeCap = 20

// senderCheckAuthenticated is what the card shows for mail whose sender's
// domain vouched for the From address.
const senderCheckAuthenticated = "authenticated"

// capturedEmployerSignal is the mail an ensure is filing, as evidence for the
// company its domain names.
func capturedEmployerSignal(in EnsureCounterpartyInput, domain string, contact ids.ContactID, company ids.CompanyID) employerSignal {
	return employerSignal{contact: contact, company: company, activity: in.ActivityID.UUID, domain: owndomain.Fold(domain)}
}

// offerEmployerChangeTx asks the owner whether the contact moved to the
// signal's company, when the signal contradicts the current primary employer.
func (s *Store) offerEmployerChangeTx(ctx context.Context, tx pgx.Tx, signal employerSignal) error {
	if s.employerChanges == nil || signal.activity.IsZero() {
		return nil
	}
	if signal.domain != "" {
		ours, err := s.domainNamesNoEmployer(ctx, tx, signal.domain)
		if err != nil || ours {
			return err
		}
	}
	change, found, err := employerChangeFor(ctx, tx, signal)
	if err != nil || !found {
		return err
	}
	return s.employerChanges.ProposeEmployerChangesTx(ctx, tx, []approvalsubject.EmployerChange{change})
}

// employerChangeFor is the question one signal raises, once its domain is
// known to be a business one.
func employerChangeFor(ctx context.Context, tx pgx.Tx, signal employerSignal) (approvalsubject.EmployerChange, bool, error) {
	// The question first: most mail comes from the employer on record, and
	// that answer costs one statement.
	change, found, err := readEmployerChange(ctx, tx, signal)
	if err != nil || !found {
		return change, false, err
	}
	if seat, err := contactIsSeatTx(ctx, tx, signal.contact.UUID); err != nil || seat {
		return change, false, err
	}
	sees, err := ownerSeesCompanies(ctx, tx, change)
	if err != nil || !sees {
		return change, false, err
	}
	change.Domain, change.Signature = signal.domain, signal.signature
	return change, true, nil
}

// domainNamesNoEmployer reports a consumer domain or one of our own.
func (s *Store) domainNamesNoEmployer(ctx context.Context, tx pgx.Tx, domain string) (bool, error) {
	consumer, err := s.consumerMailMatcher(ctx, tx)
	if err != nil || consumer.IsConsumer(domain) {
		return true, err
	}
	own, err := owndomain.Read(ctx, tx, auth.TrustedOwnDomainsSQL)
	if err != nil {
		return false, err
	}
	return own.CoversDomain(domain), nil
}

// ownerSeesCompanies reports whether the owner may open both companies the
// card names. A card never shows its reader a company they could not open.
func ownerSeesCompanies(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) (bool, error) {
	seat, err := auth.SeatPrincipal(ctx, tx, change.OwnerID)
	if err != nil {
		return false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	named := arg([]ids.UUID{change.FromCompanyID, change.ToCompanyID})
	visible := auth.VisiblePredicate(seat, companyEntity, arg)("")
	var seen int
	if err := tx.QueryRow(ctx, storekit.SQLf(`SELECT count(*) FROM company WHERE id = ANY($%d) AND %s`, named, visible),
		args...).Scan(&seen); err != nil {
		return false, fmt.Errorf("contacts: reading whether the owner may open the companies a question names: %w", err)
	}
	return seen == 2, nil
}

// employerChangeSQL reads the question for one signal, or no row when there is
// none to ask. $1 is the contact, $2 the company, $3 the mail, $4 the clock
// skew, $5 the signal's domain or ”.
//
// The mail is inbound, its sender's domain vouched for it, and its From is an
// address of this contact on the signal's domain. The owner may read it. A
// header dated ahead of our clock is forged. The mail postdates each date the
// current employment carries, its creation included, and any mail the contact
// sent from the employer's domains.
var employerChangeSQL = `
SELECT p.owner_id, p.full_name, cur.id, cur.company_id, curco.display_name, target.display_name, a.occurred_at,
       sender.address
  FROM contact p
  JOIN relationship cur ON cur.contact_id = p.id AND ` + employment.LiveCurrentPrimarySQL("cur") + `
  JOIN company curco ON curco.id = cur.company_id
  JOIN company target ON target.id = $2
  JOIN activity a ON a.id = $3 AND a.archived_at IS NULL AND a.direction = 'inbound' AND a.inbound_sender_authenticated
  JOIN LATERAL (SELECT fp.address FROM activity_participant fp
                  JOIN contact_email fe ON fe.contact_id = p.id AND fe.archived_at IS NULL AND fe.email = lower(fp.address)
                 WHERE fp.activity_id = a.id AND fp.role = 'from'
                   AND ($5 = '' OR ` + owndomain.CoversSQL("fe.email_domain_folded", "$5") + `)
                 ORDER BY fp.address LIMIT 1) sender ON true
 WHERE p.id = $1 AND p.archived_at IS NULL AND p.merged_into_id IS NULL AND p.owner_id IS NOT NULL
   AND target.archived_at IS NULL AND target.merged_into_id IS NULL AND NOT target.is_anchor
   AND ` + SignatureSourceEligible("p", "a") + `
   AND ` + occurredday.BegunSQL("a", "now() + $4::interval") + `
   AND a.occurred_at > -- occurred_at raw: captured mail is an instant, ordered against the employment's own stamps
       greatest(cur.created_at, cur.first_observed_at, cur.started_at::timestamp AT TIME ZONE 'UTC')
   AND NOT EXISTS (SELECT 1 FROM relationship held
                    WHERE held.contact_id = p.id AND held.company_id = target.id AND ` + employment.LiveSlotSQL("held") + `)
   AND NOT EXISTS (
	SELECT 1 FROM contact_email pe
	  JOIN company_domain cd ON cd.company_id = cur.company_id AND cd.archived_at IS NULL
	   AND ` + owndomain.CoversSQL("pe.email_domain_folded", "cd.domain_folded") + `
	  JOIN activity_participant ap ON lower(ap.address) = pe.email AND ap.role = 'from'
	  JOIN activity ev ON ev.id = ap.activity_id
	 WHERE pe.contact_id = p.id AND (ev.id = a.id OR ` + occurredday.AnswersSQL("ev", "a") + `))`

func readEmployerChange(ctx context.Context, tx pgx.Tx, signal employerSignal) (approvalsubject.EmployerChange, bool, error) {
	change := approvalsubject.EmployerChange{
		ContactID: signal.contact.UUID, ToCompanyID: signal.company.UUID, ActivityID: signal.activity,
		SenderCheck: senderCheckAuthenticated,
	}
	var observed time.Time
	err := tx.QueryRow(ctx, employerChangeSQL, signal.contact, signal.company, signal.activity, mailClockSkewAllowance,
		signal.domain).Scan(
		&change.OwnerID, &change.ContactName, &change.FromRelationshipID, &change.FromCompanyID,
		&change.FromCompanyName, &change.ToCompanyName, &observed, &change.FromAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		return change, false, nil
	}
	if err != nil {
		return change, false, fmt.Errorf("contacts: reading whether %s moved employer: %w", signal.contact, err)
	}
	change.ObservedOn = observed.UTC().Format(time.DateOnly)
	return change, true, nil
}

// plantDomainEmploymentAndAsk employs a settled domain's contacts who have no
// employer, and asks the owners of those who work somewhere else.
func (s *Store) plantDomainEmploymentAndAsk(ctx context.Context, tx pgx.Tx, domain string, companyID ids.CompanyID) (int, error) {
	planted, err := plantDomainEmployment(ctx, tx, domain, companyID)
	if err != nil {
		return 0, err
	}
	return planted, s.offerDomainEmployerChangesTx(ctx, tx, domain, companyID)
}

// offerDomainEmployerChangesTx asks about the contacts on a just-settled domain
// whose current employer is another company, up to domainEmployerChangeCap.
// The planter employed the rest. Each contact's newest mail from the domain is
// the evidence. The domain is judged once, and the inbox is asked once.
func (s *Store) offerDomainEmployerChangesTx(ctx context.Context, tx pgx.Tx, domain string, companyID ids.CompanyID) error {
	if s.employerChanges == nil {
		return nil
	}
	folded := owndomain.Fold(domain)
	if ours, err := s.domainNamesNoEmployer(ctx, tx, folded); err != nil || ours {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, newest.id FROM contact p
		  JOIN relationship cur ON cur.contact_id = p.id AND cur.company_id <> $2 AND `+employment.LiveCurrentPrimarySQL("cur")+`
		  JOIN LATERAL (SELECT ev.id FROM contact_email pe
		                  JOIN activity_participant ap ON lower(ap.address) = pe.email AND ap.role = 'from'
		                  JOIN activity ev ON ev.id = ap.activity_id AND ev.archived_at IS NULL
		                   AND ev.direction = 'inbound' AND ev.inbound_sender_authenticated
		                 WHERE pe.contact_id = p.id AND pe.archived_at IS NULL
		                   AND `+owndomain.CoversSQL("pe.email_domain_folded", "$1")+`
		                 ORDER BY ev.occurred_at DESC, ev.id DESC LIMIT 1) newest ON true
		 WHERE `+domainEmploymentCandidateSQL("p", "$1")+`
		 ORDER BY p.id
		 LIMIT $3`, folded, companyID, domainEmployerChangeCap)
	if err != nil {
		return fmt.Errorf("contacts: reading the newest mail on %s: %w", domain, err)
	}
	signals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (employerSignal, error) {
		signal := employerSignal{company: companyID, domain: folded}
		return signal, row.Scan(&signal.contact, &signal.activity)
	})
	if err != nil {
		return fmt.Errorf("contacts: reading the newest mail on %s: %w", domain, err)
	}
	var changes []approvalsubject.EmployerChange
	for _, signal := range signals {
		change, found, err := employerChangeFor(ctx, tx, signal)
		if err != nil {
			return err
		}
		if found {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return s.employerChanges.ProposeEmployerChangesTx(ctx, tx, changes)
}

// offerSignatureEmployerTx asks when a newer signature names one company we
// hold under that name. A name no company has, or two companies share, asks
// nothing: a signature never creates or picks a company.
func (s *Store) offerSignatureEmployerTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, activity ids.UUID, f SignatureField) error {
	if s.employerChanges == nil {
		return nil
	}
	// A pass that may not read companies asks nothing rather than failing the
	// signature it is applying.
	matches, err := EmployerNameMatchesTx(ctx, tx, f.Value)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil || len(matches) != 1 {
		return err
	}
	return s.offerEmployerChangeTx(ctx, tx, employerSignal{
		contact: contactID, company: matches[0], activity: activity, signature: f.Evidence,
	})
}

func isCompanyName(f SignatureField) bool { return f.Name == fieldCompanyName }
