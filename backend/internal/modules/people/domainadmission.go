// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Whether a domain may have a company AT ALL — a standing decision, held apart
// from the crawl that would answer what company it is.
//
// The triage beside this file asks "what does this domain's site say"; these
// functions answer a question no amount of crawling can settle. A vendor the
// business merely uses has a real corporate website, so every piece of evidence
// says "company" and only a decision says otherwise. That makes admission a
// property of the DOMAIN rather than of any sender or any read, which is why it
// outlives both.
//
// A human decision is sticky by design: an admin who lets a domain in keeps it
// in, however much bulk mail arrives afterwards.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The standing admission a domain can carry, independent of any one sender.
const (
	// DomainSuppressed refuses a domain a company outright — a vendor or
	// service the business uses rather than sells to. Every evidence test says
	// "company" for these; the point is that the workspace does not want one.
	DomainSuppressed = "suppressed"
	// DomainAdmitted is a human's override, and it is STICKY: no later verdict
	// or heuristic may re-suppress a domain a person deliberately let in.
	DomainAdmitted = "admitted"
)

// What decided an admission.
const (
	AdmissionSourceVerdict   = "verdict"
	AdmissionSourceHeuristic = "heuristic"
	AdmissionSourceHuman     = "human"
)

// domainSuppressedTx reports whether a domain carries a standing refusal.
//
// A human admission always wins: the column can only hold one value, and
// SuppressDomain refuses to overwrite 'admitted', so reading suppression here
// needs no second check for it.
func domainSuppressedTx(ctx context.Context, tx pgx.Tx, domain string) (bool, error) {
	var suppressed bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM organization_domain_disposition
		   WHERE domain = $1 AND admission = $2)`, domain, DomainSuppressed).Scan(&suppressed)
	if err != nil {
		return false, fmt.Errorf("people: reading the admission of %s: %w", domain, err)
	}
	return suppressed, nil
}

// SetDomainAdmission records a standing decision about a domain: refuse it a
// company, or admit it and keep it admitted.
//
// A human decision is STICKY. An admin who unblocks mckinsey.com because it
// became a client must not find it re-suppressed by the next newsletter that
// arrives from it, so an automatic caller may not overwrite a human's row —
// the guard is on the SOURCE already stored, not on the value being written.
// A human may always overwrite anything, including another human's decision.
//
// The row is created if the domain has never been seen, because an admin
// blocking a competitor's newsletter before it ever arrives is the same
// decision as blocking one that already did.
// The source is NOT a parameter: this method IS the human path, the one the
// admin surface calls, and it takes the organization-update gate above. A
// caller-supplied source would let any code claim human authority and make its
// decision sticky, which is the one thing the stickiness rule exists to stop.
// Machine callers use SuppressBulkSenderDomainTx, which stamps its own source.
func (s *Store) SetDomainAdmission(ctx context.Context, domain, admission, reason string) (BlockedDomain, error) {
	if admission != DomainSuppressed && admission != DomainAdmitted {
		return BlockedDomain{}, fmt.Errorf("people: %q is not a domain admission", admission)
	}
	if reason == "" {
		return BlockedDomain{}, errors.New("people: a domain admission needs a reason a human can read")
	}
	// Blocking a domain decides that no company will ever exist for it, and
	// unblocking one lets the next message create it. Both are the organization
	// object's own write authority, so both take the same gate a create does.
	if err := auth.Require(ctx, entityOrganization, principal.ActionUpdate); err != nil {
		return BlockedDomain{}, err
	}
	base, ok := freemail.Hostname(domain)
	if !ok {
		return BlockedDomain{}, fmt.Errorf("people: %q is not a domain", domain)
	}
	var stored BlockedDomain
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The decision this write replaces comes back FROM the write, because
		// "who unblocked this, and what was it before" is the question this
		// surface exists to answer. Without it admission_source says "human"
		// but never WHICH human, and admission_at is overwritten on every
		// change.
		before, applied, err := setDomainAdmissionTx(ctx, tx, base, admission, reason, AdmissionSourceHuman)
		if err != nil {
			return err
		}
		if !applied {
			// The sticky rule refuses a MACHINE overwrite of a human decision,
			// and this path always writes as the human — so a decision that did
			// not land means the row was never replaced, and the audit row below
			// would describe a change the table does not carry.
			return fmt.Errorf("people: the admission of %s was not recorded", base)
		}
		if admission == DomainAdmitted {
			// Unblocking has to RE-ASK, not merely clear a flag. The domain was
			// already asked and answered — that is why it is on this list — so
			// nothing would ever ask again on its own, and an admin who unblocked
			// McKinsey because they became a client would watch nothing happen.
			//
			// The owner is stamped from the acting human: triage may not mint
			// rows for a domain nobody is accountable for, and a
			// machine-suppressed row has no owner at all. An agent acting for
			// somebody carries that authority in OnBehalfOf, so the same
			// resolution capture uses applies here — the row records the human
			// who is answerable, never the machine that typed it.
			if err := reopenAdmittedDomainTx(ctx, tx, base, actingHuman(ctx)); err != nil {
				return err
			}
		}
		stored, err = readDomainAdmissionTx(ctx, tx, base)
		if err != nil {
			return err
		}
		// Audit-only (EVT-NOEVT-3): capture posture is not a record change the
		// event stream carries, but it IS a decision somebody must answer for.
		after := map[string]any{
			auditKeyDomain: stored.Domain, "admission": stored.Admission,
			"admission_reason": stored.Reason, "admission_source": stored.Source,
		}
		// A first decision replaces nothing: there was no admission, no reason
		// and nobody answerable for one. A later decision moved all three, and
		// says what they were — which is the question this surface exists for.
		if before == nil {
			_, auditErr := storekit.AuditEvent(ctx, tx, "update", entityOrganization, stored.ID, after)
			return auditErr
		}
		_, auditErr := storekit.Audit(ctx, tx, "update", entityOrganization, stored.ID, before, after)
		return auditErr
	})
	if err != nil {
		return BlockedDomain{}, err
	}
	return stored, nil
}

// setDomainAdmissionTx is SetDomainAdmission on the caller's transaction, for
// the verdict engine, which must record the refusal in the same commit as the
// verdict that concluded it.
//
// It answers the decision it REPLACED — nil when the domain carried none — and
// whether the sticky rule let this write through at all.
//
// The prior decision is read by the upsert ITSELF, in a CTE, under a lock on
// the domain. The image decides which audit door the caller takes, so a second
// decision committing between a separate read and this write would leave one
// row claiming there was nothing before. The CTE alone does not close that: under
// READ COMMITTED the ON CONFLICT re-check resolves against a row committed after
// the CTE's snapshot was taken, so the lock is what makes the snapshot the same
// answer the upsert acts on.
func setDomainAdmissionTx(ctx context.Context, tx pgx.Tx, domain, admission, reason, source string) (map[string]any, bool, error) {
	if err := lockDomainAdmissionTx(ctx, tx, domain); err != nil {
		return nil, false, err
	}
	// A domain nobody has decided about carries a row with a NULL admission —
	// the triage opens one on first contact — so the image is built from the
	// admission itself rather than from the row's existence.
	var priorJSON []byte
	err := tx.QueryRow(ctx, `
		WITH was AS (
		  SELECT admission, admission_reason, admission_source
		    FROM organization_domain_disposition WHERE domain = $1
		)
		INSERT INTO organization_domain_disposition (domain, status, admission, admission_reason, admission_source, admission_at)
		VALUES (
		        $1, $2, $3, $4, $5, now())
		ON CONFLICT (domain) DO UPDATE
		   SET admission = EXCLUDED.admission,
		       admission_reason = EXCLUDED.admission_reason,
		       admission_source = EXCLUDED.admission_source,
		       admission_at = now(),
		       -- A refused domain is not waiting on evidence; it has an answer.
		       -- Only a suppression clears the marker, because admitting one
		       -- leaves the company question genuinely open again.
		       pending_reason = CASE WHEN EXCLUDED.admission = 'suppressed'
		                             THEN NULL ELSE organization_domain_disposition.pending_reason END,
		       -- next_attempt_at means scheduled work, and a refused domain has
		       -- none: nothing will crawl it while the refusal stands.
		       next_attempt_at = CASE WHEN EXCLUDED.admission = 'suppressed'
		                              THEN NULL ELSE organization_domain_disposition.next_attempt_at END,
		       updated_at = now()
		 -- The sticky rule: an automatic caller may not overwrite a decision a
		 -- HUMAN made, while a human may overwrite anything. Guarded on the
		 -- source already stored, not on the value being written, because what
		 -- must survive is the authority behind the row rather than which way
		 -- it happened to point.
		 WHERE organization_domain_disposition.admission_source IS DISTINCT FROM $6
		    OR EXCLUDED.admission_source = $6
		RETURNING (SELECT jsonb_build_object(
		             'admission', was.admission,
		             'admission_reason', coalesce(was.admission_reason, ''),
		             'admission_source', coalesce(was.admission_source, ''))
		             FROM was WHERE was.admission IS NOT NULL)`,
		domain, DomainPending, admission, reason, source, AdmissionSourceHuman).Scan(&priorJSON)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// The sticky rule declined this write: a machine may not overwrite a
		// human's decision. Nothing moved, so there is no prior state to name.
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("people: recording the admission of %s: %w", domain, err)
	case priorJSON == nil:
		return nil, true, nil
	}
	var prior map[string]any
	if err := json.Unmarshal(priorJSON, &prior); err != nil {
		return nil, false, fmt.Errorf("people: reading the decision %s carried before: %w", domain, err)
	}
	return prior, true, nil
}

// lockDomainAdmissionTx serializes every writer of one domain's admission, so
// the decision a write replaces is read under the same lock that replaces it.
func lockDomainAdmissionTx(ctx context.Context, tx pgx.Tx, domain string) error {
	return storekit.LockWriteIdentity(ctx, tx, admissionWriteIdentity, domain)
}

// admissionWriteIdentity names the write identity a domain's admission
// decisions serialize on.
const admissionWriteIdentity = "domain_admission"

// SuppressBulkSenderDomainTx refuses a domain a company because its mail was
// judged bulk or automated, on the caller's transaction so the refusal commits
// with the verdict that concluded it.
//
// A consumer-mail domain is skipped. Nobody's employer is gmail.com, so there
// is no company to refuse, and recording one would put a mail provider in the
// admin's blocked list as though somebody had decided something about it. The
// workspace's own carve-outs travel with the check, exactly as they do in the
// ensure path — an admin who declared a shared domain corporate must see it
// judged that way on both sides.
func (s *Store) SuppressBulkSenderDomainTx(ctx context.Context, tx pgx.Tx, domain, reason string) error {
	base, ok := freemail.Hostname(domain)
	if !ok {
		return nil
	}
	consumerMail, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return err
	}
	if consumerMail.IsConsumer(base) {
		return nil
	}
	// The sticky rule may decline this write, which is the point of it: a
	// human's decision stands. The verdict engine records its own conclusion
	// either way, so which happened is not this caller's to report.
	_, _, err = setDomainAdmissionTx(ctx, tx, base, DomainSuppressed, reason, AdmissionSourceVerdict)
	return err
}

// reopenWithheldDispositionTx puts a withheld domain back in the sweep's path.
//
// A domain marked unevidenced has no next_attempt_at, which is what stops it
// being crawled over and over on evidence that is not going to improve on its
// own. New mail IS evidence that it might: somebody there is still writing, so
// the site is worth another look. This also repairs the two ways such a row can
// be stranded — a machine-created suppression row has no owner, and an owner is
// what the triage needs before it may mint anything.
//
// Guarded on the withheld state, so it cannot disturb a question already in
// flight, and never touches a SUPPRESSED domain: a refusal is not a question
// waiting for evidence.
func reopenWithheldDispositionTx(ctx context.Context, tx pgx.Tx, domain string, ownerID ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE organization_domain_disposition
		   SET pending_reason = NULL,
		       attempts = 0,
		       next_attempt_at = now(),
		       owner_id = COALESCE(owner_id, $2),
		       updated_at = now()
		 WHERE domain = $1
		   AND status = $3
		   AND admission IS DISTINCT FROM $4
		   AND (pending_reason = 'unevidenced' OR next_attempt_at IS NULL OR owner_id IS NULL)`,
		domain, ownerID, DomainPending, DomainSuppressed); err != nil {
		return fmt.Errorf("people: reopening the withheld question for %s: %w", domain, err)
	}
	return nil
}

// admitClaimedDomainTx lifts a standing refusal because a human deliberately
// claimed the domain for a company they are creating or editing.
//
// It writes only when a suppression is actually there, so an ordinary create on
// an ordinary domain adds no row and leaves the admin's blocked list showing
// only decisions somebody made. A domain never refused needs no admission.
func admitClaimedDomainTx(ctx context.Context, tx pgx.Tx, domain string) error {
	base, ok := freemail.Hostname(domain)
	if !ok {
		return nil
	}
	// Only a PERSON's claim lifts a refusal. Creating an organization is a 🟢
	// auto-execute agent tool, so stamping the source as human unconditionally
	// would let an agent launder a machine decision into one the sticky rule
	// then protects for ever — defeating the guard by writing the word it
	// checks for. An agent's claim creates the company and leaves the standing
	// refusal exactly as the human left it.
	if !claimedByHuman(ctx) {
		return nil
	}
	// The same lock the decision writer takes: this rewrites the admission a
	// concurrent decision is reading as its before-image.
	if err := lockDomainAdmissionTx(ctx, tx, base); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_domain_disposition
		   SET admission = $2, admission_source = $3, admission_at = now(),
		       admission_reason = 'a person put this domain on a company they created',
		       updated_at = now()
		 WHERE domain = $1 AND admission = $4`,
		base, DomainAdmitted, AdmissionSourceHuman, DomainSuppressed); err != nil {
		return fmt.Errorf("people: admitting the claimed domain %s: %w", base, err)
	}
	return nil
}

// reopenAdmittedDomainTx puts a just-admitted domain back in the triage sweep's
// path, so the company question a refusal closed gets asked again.
//
// It re-opens every state EXCEPT 'company'. A domain that already has its
// company needs no second answer and its people are already employed there —
// but 'personal', 'provider' and 'no_site' are settled WITHOUT one, and those
// are exactly the domains an admin unblocks. Guarding on 'pending' alone left
// them matching zero rows: the admin's decision recorded, and nothing else
// happening, which is the failure this function exists to prevent.
//
// Setting the status back to pending is what the sweep reads; clearing the
// evidence is what stops the old refusal being shown as the reason for a
// question that is open again.
func reopenAdmittedDomainTx(ctx context.Context, tx pgx.Tx, domain string, ownerID *ids.UUID) error {
	// owner_id has a foreign key to app_user, so it is a POINTER: a zero uuid
	// would fail that constraint rather than record "nobody", and NULL is the
	// honest spelling of an owner we do not have.
	if _, err := tx.Exec(ctx, `
		UPDATE organization_domain_disposition
		   SET status = $4,
		       pending_reason = NULL,
		       evidence = NULL,
		       attempts = 0,
		       next_attempt_at = now(),
		       owner_id = COALESCE(owner_id, $2),
		       updated_at = now()
		 WHERE domain = $1 AND admission = $3 AND status <> $5`,
		domain, ownerID, DomainAdmitted, DomainPending, DomainCompany); err != nil {
		return fmt.Errorf("people: re-opening the company question for %s: %w", domain, err)
	}
	return nil
}

// readDomainAdmissionTx reads back what was actually stored, which is not what
// the caller sent: the domain is normalized to its registrable form and the
// decision time is the database's.
func readDomainAdmissionTx(ctx context.Context, tx pgx.Tx, domain string) (BlockedDomain, error) {
	var d BlockedDomain
	var orgID *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id, domain, COALESCE(admission, ''), COALESCE(admission_reason, ''),
		       COALESCE(admission_source, ''), admission_at, organization_id
		  FROM organization_domain_disposition WHERE domain = $1`, domain).
		Scan(&d.ID, &d.Domain, &d.Admission, &d.Reason, &d.Source, &d.DecidedAt, &orgID)
	if err != nil {
		return BlockedDomain{}, fmt.Errorf("people: reading back the admission of %s: %w", domain, err)
	}
	if orgID != nil {
		typed := ids.From[ids.OrganizationKind](*orgID)
		d.OrganizationID = &typed
	}
	return d, nil
}

// actingHuman is the app_user answerable for this call, or nil when none is —
// an agent or connector carries that authority in OnBehalfOf, a human call in
// UserID, and a system principal has neither.
func actingHuman(ctx context.Context) *ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil
	}
	owner := actor.OnBehalfOf
	if owner.IsZero() {
		owner = actor.UserID
	}
	if owner.IsZero() {
		return nil
	}
	return &owner
}

// claimedByHuman reports whether this call is a person acting directly, rather
// than an agent or connector acting on their authority.
//
// The distinction matters only where a decision becomes STICKY: an agent may do
// plenty on somebody's behalf, but it may not record a judgement that later
// machine decisions are then forbidden to revisit.
func claimedByHuman(ctx context.Context) bool {
	// A BULK write is not a person at a form, whoever approved it. Someone who
	// approved a file of twelve thousand companies did not make twelve thousand
	// judgements about domains capture had refused, and the sentence this lift
	// writes — "a person put this domain on a company they created" — would not
	// be true of any of them. The import says so on its way in; every other
	// caller leaves the marker unset and is unaffected.
	if IsBulkWrite(ctx) {
		return false
	}
	actor, ok := principal.Actor(ctx)
	// PrincipalHuman is the field that answers this, and it is what
	// auth.RequireHuman reads. OnBehalfOf looked like the same question and is
	// not: several genuine human paths set it to the caller's own id, so
	// testing it would refuse a refusal-lift to a person who really did claim
	// the domain.
	return ok && actor.Type == principal.PrincipalHuman && !actor.UserID.IsZero()
}

// bulkWriteKey marks a write as one of many, made under a single approval.
type bulkWriteKey struct{}

// WithBulkWrite marks this context as a bulk write: many records landing under
// one human's single decision, rather than one record a human is looking at.
//
// It exists for one question — whether a claim is strong enough to lift a
// standing domain refusal — and answers it no. A refusal is a decision somebody
// made about one domain, and only an equally specific claim should overturn it.
// Importing a spreadsheet is not that, however senior the person who approved
// the run.
func WithBulkWrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, bulkWriteKey{}, true)
}

// IsBulkWrite reports whether this write is part of a bulk operation.
func IsBulkWrite(ctx context.Context) bool {
	marked, _ := ctx.Value(bulkWriteKey{}).(bool)
	return marked
}
