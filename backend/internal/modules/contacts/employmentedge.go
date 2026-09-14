// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Attaching a captured contact to a company: whether the company is still one
// records may be attached to, and the edge itself.
//
// Both halves are here because both are about the same rule — capture SUGGESTS
// an employer and never reassigns one — and because the ensure ladder reaches
// them from two places now: the ordinary attach, and the re-look a capture does
// after losing a race with a triage verdict.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attachToSettledVerdict handles a domain answered WHILE this ensure was
// running. Taking the disposition lock is what ordered the two, and the dedupe
// that said "no company" ran before it — so a verdict that committed in
// between created a company this path has not seen. Attaching here is the
// difference between the contact getting their employer now and waiting for
// their next message.
//
// A refusal attaches nothing, and neither does a company that has since been
// archived: the ledger names a company, it does not promise one is still
// there to join.
func attachToSettledVerdict(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, prior DomainDisposition, res *EnsureCounterpartyResult) error {
	if prior.CompanyID == nil {
		return nil
	}
	live, err := companyIsLive(ctx, tx, *prior.CompanyID)
	if err != nil || !live {
		return err
	}
	res.CompanyID = prior.CompanyID
	return plantEmploymentEdge(ctx, tx, in, res.ContactID, *prior.CompanyID)
}

// companyIsLive reports whether a company is still one records may be
// attached to — neither archived nor merged away — locking the row so the answer
// cannot change under the insert that follows it.
func companyIsLive(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, `
		SELECT archived_at IS NULL AND merged_into_id IS NULL
		  FROM company WHERE id = $1 FOR UPDATE`, companyID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: checking whether company %s is still live: %w", companyID, err)
	}
	return live, nil
}

// plantEmploymentEdge attaches one contact to one company, and only when they
// have no current primary employer: capture SUGGESTS an employer, it never
// reassigns somebody's. TWO partial uniques are the structural guard — the
// current-primary one and uq_rel_employment, which refuses a second live edge
// for the same pair — and ON CONFLICT DO NOTHING is what makes either of them
// a no-op here rather than a failure: capture has nothing to add to an
// employment that already exists. The NOT EXISTS keeps a concurrent race with
// the first from surfacing as a 500.
func plantEmploymentEdge(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, contactID ids.ContactID, companyID ids.CompanyID) error {
	// The employment edge hangs off the contact, so an archive in flight must
	// not be outrun — see lockContactForAttach.
	if err := lockContactForAttach(ctx, tx, contactID); err != nil {
		return err
	}
	// And the same per-contact lock every other writer of this contact's
	// current-primary employment takes. The contact row lock above answers a
	// different question — is this contact still here — and the two writers that
	// decide the flag WITHOUT one (update, archive) take only this. Without it
	// capture reads "they already have a primary", plants nothing, and a patch
	// ending that employment commits beside it: the contact is left employed
	// once, unmarked, which is the state the flag exists to make impossible.
	//
	// After the row lock, never before: create takes them in this order too, and
	// two writers taking one pair of locks in opposite orders is a deadlock.
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contactID.String()); err != nil {
		return err
	}
	var edgeID ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO relationship (kind, contact_id, company_id, is_current_primary, source, captured_by)
		SELECT 'employment', $1, $2, true, $3, $4
		WHERE NOT EXISTS (
			SELECT 1 FROM relationship
			WHERE contact_id = $1 AND `+employment.CurrentPrimarySlotSQL("")+`)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		contactID, companyID, in.Source, in.CapturedBy).Scan(&edgeID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either guard skipped it: the contact already has a current primary
		// employer, or this exact edge already exists. Nothing was written, so
		// nothing is audited and nothing is published — a no-op must not mint
		// history.
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: insert employment edge: %w", err)
	}
	return auditCapturedEmployment(ctx, tx, edgeID, contactID, companyID, relationshipOriginCapture)
}

// adoptDispositionForCompany settles a domain onto the company that already
// exists for it. It is the one thing that reverses a triage refusal: a wrong
// `personal` verdict is otherwise permanent, and the only correction available
// to a human is to create the company themselves — which this makes stick.
//
// A no-op for a domain nobody ever asked about, so the ordinary case of mail
// arriving at a long-known company writes nothing.
func adoptDispositionForCompany(ctx context.Context, tx pgx.Tx, domain string, companyID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET status = $2, source = $3, company_id = $4,
		       evidence = 'a human put a company on this domain',
		       next_attempt_at = NULL, updated_at = now()
		 WHERE domain = $1
		   AND (status <> $2 OR company_id IS DISTINCT FROM $4)
		   -- Only a REFUSAL is a human's to overturn. A no_site row already
		   -- carrying the company triage itself created is not: claiming
		   -- that as "a human put a company on this domain" would rewrite the
		   -- provenance of a machine decision nobody overruled, and the
		   -- evidence field exists to say why a company was refused.
		   AND NOT (source = $5 AND company_id IS NOT DISTINCT FROM $4)`,
		domain, DomainCompany, DomainSourceHuman, companyID, DomainSourceHeuristic); err != nil {
		return fmt.Errorf("contacts: recording that a human settled %s: %w", domain, err)
	}
	return nil
}

// deferCompanyToTriage handles a domain with no company behind it yet: decide
// whether the question is even worth asking, and if it is, open it.
//
// Nothing is created here on purpose. The contact is already committed and the
// message is already on their timeline; what is withheld is a company row that
// nothing yet justifies. ADR-0063's create-on-sight is what manufactured
// "Kestner" from a man's own domain, and no later evidence removed it.
func (s *Store) deferCompanyToTriage(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, base string, res *EnsureCounterpartyResult) error {
	// Consumer mail is answered by the domain itself and needs no crawl to
	// settle. The sink's tier ladder usually catches it first; this repeats the
	// check because the verdict engine and the review-queue accept reach this
	// same chokepoint without passing through that ladder.
	consumerMail, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return err
	}
	if consumerMail.IsConsumer(base) {
		return nil
	}
	prior, known, err := readDispositionTx(ctx, tx, base)
	if err != nil {
		return err
	}
	if known && prior.Settled() {
		return attachToSettledVerdict(ctx, tx, in, prior, res)
	}
	if known {
		// The question is open. Usually somebody already asked it and the crawl
		// is coming, so this message adds nothing — but a WITHHELD domain is
		// open with its cursor cleared, which means no sweep will ever offer it
		// again. New mail is new evidence, so it rearms the question: a company
		// whose site was down when we first looked gets another chance the next
		// time somebody there writes, instead of waiting for a human forever.
		return reopenWithheldDispositionTx(ctx, tx, base, in.OwnerID)
	}
	opened, err := recordPendingDispositionTx(ctx, tx, base, in.OwnerID)
	if err != nil {
		return err
	}
	res.TriagePending, res.TriageDomain = opened, base
	return nil
}

// auditCapturedEmployment records an employment CAPTURE planted, and tells the
// bus about it — the write shape's other two rows, in the same transaction as
// the edge.
//
// The envelope is the anchor's own contact.updated, exactly as a human-created
// relationship uses, because that is what a relationship change publishes here:
// there is no separate "employment created" kind to collide with. What the
// delta carries that a human's does not is `origin: "capture"`. A consumer that
// treats an employer as a fact somebody asserted can then tell an inference
// from a statement, which is the whole difference between the two paths — and
// it can do so without every existing consumer of contact.updated changing.
func auditCapturedEmployment(ctx context.Context, tx pgx.Tx, edgeID ids.UUID, contactID ids.ContactID, companyID ids.CompanyID, origin string) error {
	auditID, err := storekit.Audit(ctx, tx, actionCreate, "relationship", edgeID, nil, map[string]any{
		relationshipKindField: employmentKind, "origin": origin,
	})
	if err != nil {
		return fmt.Errorf("contacts: audit the captured employment edge: %w", err)
	}
	delta := map[string]any{
		eventKeyDelta: map[string]any{"relationship": map[string]any{
			"id": edgeID, relationshipKindField: employmentKind, "action": actionCreate,
			"company_id": companyID, "origin": origin,
		}},
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID,
		relationshipUpdatedPayload("contact", delta)); err != nil {
		return fmt.Errorf("contacts: publish the captured employment edge: %w", err)
	}
	return nil
}

// relationshipOriginCapture marks an edge the capture path inferred rather than
// one a human asserted. Spelled once because the audit row and the event delta
// must agree — a reader comparing the two would otherwise have to decide which
// spelling was authoritative.
const relationshipOriginCapture = "capture"

// relationshipOriginProvider marks an edge a licensed data provider asserted:
// somebody was PAID to say this contact works there. Distinct from capture,
// which inferred it from correspondence the installation already had, because
// the two carry different weight and a reader deciding whether to trust an
// employer must be able to tell them apart.
const relationshipOriginProvider = "provider"

// employmentKind is the relationship kind this file plants, spelled once so the
// SQL, the audit row and the event delta cannot drift apart.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const employmentKind = "employment"
