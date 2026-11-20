// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Noticing that a contact moved. Capture never reassigns an employer
// (employmentedge.go), so this asks the contact's owner instead.
//
// The sender picks the From address and writes the signature. So the evidence
// must name an existing company, and be newer than all that places the contact
// at the employer we hold. Consumer domains, our own domains and our own
// company never count.

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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmployerChangeProposer puts an employer change to the contact's owner, in the
// transaction that captured the evidence. Compose binds the approvals inbox;
// nil asks nobody.
type EmployerChangeProposer interface {
	ProposeEmployerChangeTx(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) error
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
// through its mail domain, or through the company its signature names.
type employerSignal struct {
	contact   ids.ContactID
	company   ids.CompanyID
	activity  ids.UUID
	domain    string
	signature string
}

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
	// The question first: most mail comes from the employer on record, and
	// that answer costs one statement.
	change, found, err := readEmployerChange(ctx, tx, signal)
	if err != nil || !found {
		return err
	}
	if signal.domain != "" {
		ours, err := s.domainNamesNoEmployer(ctx, tx, signal.domain)
		if err != nil || ours {
			return err
		}
	}
	if seat, err := contactIsSeatTx(ctx, tx, signal.contact.UUID); err != nil || seat {
		return err
	}
	change.Domain, change.Signature = signal.domain, signal.signature
	return s.employerChanges.ProposeEmployerChangeTx(ctx, tx, change)
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

// employerChangeSQL reads the question for one signal, or no row when there is
// none to ask. $1 is the contact, $2 the company, $3 the mail, $4 the clock skew.
//
// A header dated ahead of our clock is forged. The mail must postdate each date
// the current employment carries, its creation included. A backfill reads old
// mail after the record was set up, and an old address must not undo it. The
// mail must also be newer than any the contact sent from the employer's domains.
var employerChangeSQL = `
SELECT p.owner_id, p.full_name, cur.id, cur.company_id, curco.display_name, target.display_name, a.occurred_at
  FROM contact p
  JOIN relationship cur ON cur.contact_id = p.id AND ` + employment.LiveCurrentPrimarySQL("cur") + `
  JOIN company curco ON curco.id = cur.company_id
  JOIN company target ON target.id = $2
  JOIN activity a ON a.id = $3 AND a.restricted_at IS NULL
 WHERE p.id = $1 AND p.archived_at IS NULL AND p.merged_into_id IS NULL AND p.owner_id IS NOT NULL
   AND target.archived_at IS NULL AND target.merged_into_id IS NULL AND NOT target.is_anchor
   AND ` + occurredday.BegunSQL("a", "now() + $4::interval") + `
   AND a.occurred_at > -- occurred_at raw: captured mail is an instant, ordered against the employment's own stamps
       greatest(cur.created_at, cur.first_observed_at, cur.started_at::timestamp AT TIME ZONE 'UTC')
   AND NOT EXISTS (SELECT 1 FROM relationship held
                    WHERE held.contact_id = p.id AND held.company_id = target.id AND ` + employment.LiveSlotSQL("held") + `)
   AND NOT EXISTS (
	SELECT 1 FROM contact_email pe
	  JOIN company_domain cd ON cd.company_id = cur.company_id AND cd.archived_at IS NULL
	   AND ` + owndomain.CoversSQL("pe.email_domain_folded", "cd.domain_folded") + `
	  JOIN activity_participant ap ON lower(ap.address) = lower(pe.email) AND ap.role = 'from'
	  JOIN activity ev ON ev.id = ap.activity_id
	 WHERE pe.contact_id = p.id AND (ev.id = a.id OR ` + occurredday.AnswersSQL("ev", "a") + `))`

func readEmployerChange(ctx context.Context, tx pgx.Tx, signal employerSignal) (approvalsubject.EmployerChange, bool, error) {
	change := approvalsubject.EmployerChange{
		ContactID: signal.contact.UUID, ToCompanyID: signal.company.UUID, ActivityID: signal.activity,
	}
	var observed time.Time
	err := tx.QueryRow(ctx, employerChangeSQL, signal.contact, signal.company, signal.activity, mailClockSkewAllowance).Scan(
		&change.OwnerID, &change.ContactName, &change.FromRelationshipID, &change.FromCompanyID,
		&change.FromCompanyName, &change.ToCompanyName, &observed)
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

// offerDomainEmployerChangesTx asks about every contact on a just-settled
// domain whose current employer is another company. The planter employed the
// rest. Each contact's newest mail from the domain is the evidence.
func (s *Store) offerDomainEmployerChangesTx(ctx context.Context, tx pgx.Tx, domain string, companyID ids.CompanyID) error {
	if s.employerChanges == nil {
		return nil
	}
	folded := owndomain.Fold(domain)
	rows, err := tx.Query(ctx, `
		SELECT p.id, newest.id FROM contact p
		  JOIN relationship cur ON cur.contact_id = p.id AND cur.company_id <> $2 AND `+employment.LiveCurrentPrimarySQL("cur")+`
		  JOIN LATERAL (SELECT ev.id FROM contact_email pe
		                  JOIN activity_participant ap ON lower(ap.address) = lower(pe.email) AND ap.role = 'from'
		                  JOIN activity ev ON ev.id = ap.activity_id AND ev.restricted_at IS NULL
		                 WHERE pe.contact_id = p.id AND pe.archived_at IS NULL
		                   AND `+owndomain.CoversSQL("pe.email_domain_folded", "$1")+`
		                 ORDER BY ev.occurred_at DESC, ev.id DESC LIMIT 1) newest ON true
		 WHERE `+domainEmploymentCandidateSQL("p", "$1")+`
		 ORDER BY p.id`, folded, companyID)
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
	for _, signal := range signals {
		if err := s.offerEmployerChangeTx(ctx, tx, signal); err != nil {
			return err
		}
	}
	return nil
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

// ApplyEmployerChangeTx writes an accepted employer change as the decider. The
// new company becomes the current primary employer. The old employment ends as
// former, dated by the mail when that date is after its start. It answers false
// and writes nothing when the old employment is no longer the current one.
func (s *Store) ApplyEmployerChangeTx(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) (bool, error) {
	if err := auth.HoldWritableLive(ctx, tx, contactEntity, change.ContactID); err != nil {
		return false, err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, change.ContactID.String()); err != nil {
		return false, err
	}
	var current ids.UUID
	var started *time.Time
	err := tx.QueryRow(ctx, `SELECT r.id, r.started_at FROM relationship r WHERE r.contact_id = $1 AND `+
		employment.LiveCurrentPrimarySQL("r"), change.ContactID).Scan(&current, &started)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && current != change.FromRelationshipID) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the employment an employer change ends: %w", err)
	}
	if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, change.ToCompanyID); err != nil {
		return false, err
	}
	live, err := liveEmploymentAt(ctx, tx, change.ContactID, change.ToCompanyID)
	if err != nil {
		return false, err
	}
	if err := requireEmployerWrite(ctx, live); err != nil {
		return false, err
	}
	if err := auth.Require(ctx, tableRelationship, principal.ActionUpdate); err != nil {
		return false, err
	}
	if _, _, err := s.employAt(ctx, tx, change.ContactID, change.ToCompanyID, "", employerChangeSource); err != nil {
		return false, err
	}
	return true, s.endEmployment(ctx, tx, change, started)
}

// endEmployment marks the old employment former. The mail's day is the latest
// the old job can have ended, so it is the end date unless it falls before the
// start.
func (s *Store) endEmployment(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange, started *time.Time) error {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	former, primary := employmentFormer, false
	patch := UpdateRelationshipInput{
		EmploymentStatus: &former, IsCurrentPrimary: &primary,
		Evidence: map[string]any{auditKeySource: employerChangeSource, "activity_id": change.ActivityID},
	}
	if ended, err := time.Parse(time.DateOnly, change.ObservedOn); err == nil && (started == nil || !ended.Before(*started)) {
		patch.EndedAt = &ended
	}
	_, err = s.updateRelationshipInTx(ctx, tx, change.FromRelationshipID, patch, capturedBy)
	return err
}

// employerChangeSource is the source of an employment an accepted employer
// change starts, and the audit evidence on the one it ends.
const employerChangeSource = approvalsubject.KindEmployerChange
