// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The owning domain's half of the licensed-data-provider seam (ADR-0101):
// people decides whether a subject may be enriched at all, which other
// records might be the same human, and what may leave the installation about
// them. modules/integrations declares these as func types because that module
// cannot import this one; compose wires them together.
//
// Everything here reads the tables people owns. The consent VERDICT lives in
// a sibling module this one may not import, so the fence reads the consent
// state row directly — the same row that module's verdict is built on.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// HoldEnrichmentSubject is EnrichmentFence for the hand-off: it takes the
// subject's row lock FIRST, then asks the same question.
//
// The lock is what makes the answer survive the write that follows. The fence
// alone reads a snapshot, and under READ COMMITTED an Art. 17 erasure
// committing between that read and the claim insert lands anyway — refilling
// the very tables the erasure just cleared, since person_provider_claim and
// every record column the values reach are declared PII it DELETES.
//
// Queue time does not need this and does not take it: nothing is written about
// the subject there beyond the run row, and QueueRun already holds the subject
// through EnsureWritableLive. Holding a lock across the whole queueing
// transaction would serialize every run against every other write to that
// person for no gain.
//
// Subject-first, because the eraser is subject-first: a transaction that took
// any other row lock before this one has already lost the ordering that keeps
// the two taking turns.
func HoldEnrichmentSubject(ctx context.Context, tx pgx.Tx, personID string) (allowed bool, reason provider.SkipReason, err error) {
	id, err := ids.Parse(personID)
	if err != nil {
		return false, "", fmt.Errorf("people: the enrichment subject's id: %w", err)
	}
	if err := auth.LockSubjectLive(ctx, tx, entityPerson, id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// Erased, archived or deleted under a run already in flight.
			// The same answer the fence gives for a subject it cannot read.
			return false, provider.SkipNotEligible, nil
		}
		return false, "", err
	}
	return EnrichmentFence(ctx, tx, personID)
}

// EnrichmentFence answers whether one subject may be enriched, inside the
// caller's transaction. The two refusals are different facts and the caller
// records them differently: a suppressed subject objected, an ineligible one
// is a record we should not be buying data about at all.
//
// It runs at queue time AND again immediately before any claim is written,
// because a subject can object while a paid run is in flight (PI-AC-7).
func EnrichmentFence(ctx context.Context, tx pgx.Tx, personID string) (allowed bool, reason provider.SkipReason, err error) {
	var archived, merged bool
	err = tx.QueryRow(ctx, `
		SELECT archived_at IS NOT NULL, merged_into_id IS NOT NULL
		  FROM person WHERE id = $1`, personID).Scan(&archived, &merged)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Erased or deleted under a run already in flight. Not eligible
			// is the honest answer: there is no subject to enrich.
			return false, provider.SkipNotEligible, nil
		}
		return false, "", fmt.Errorf("people: reading the enrichment subject: %w", err)
	}
	if archived || merged {
		// An archived record is one we have stopped working, and a merged one
		// is a duplicate whose survivor is the subject. Buying data for
		// either spends money on a row nobody reads.
		return false, provider.SkipNotEligible, nil
	}

	objected, err := hasStandingObjection(ctx, tx, personID)
	if err != nil {
		return false, "", err
	}
	if objected {
		return false, provider.SkipSuppressed, nil
	}
	suppressed, err := anyAddressSuppressed(ctx, tx, personID)
	if err != nil {
		return false, "", err
	}
	if suppressed {
		return false, provider.SkipSuppressed, nil
	}
	return true, "", nil
}

// hasStandingObjection reports whether the subject has withdrawn consent for
// ANY purpose. Enrichment is not itself a contact purpose, so there is no one
// purpose to check: a person who told us to stop for any of them is a person
// whose data we do not go out and buy more of.
func hasStandingObjection(ctx context.Context, tx pgx.Tx, personID string) (bool, error) {
	var objected bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM person_consent
		   WHERE person_id = $1 AND state = 'withdrawn')`, personID).Scan(&objected); err != nil {
		return false, fmt.Errorf("people: reading the subject's consent state: %w", err)
	}
	return objected, nil
}

// anyAddressSuppressed asks the erasure-suppression list about every address
// on file.
//
// The case it covers is a LIVE record holding an address that was suppressed
// under some earlier erasure — a re-captured contact, or a colleague's import
// that reintroduced someone who had asked to be forgotten. The erased record
// itself is already refused by the archived check above and holds no
// addresses to test anyway, since erasure deletes them. Buying fresh data
// about a suppressed address is precisely the resurrection the list exists to
// prevent (A13), and only the hash can see it: the new record carries no
// memory of the old one.
func anyAddressSuppressed(ctx context.Context, tx pgx.Tx, personID string) (bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT email FROM person_email WHERE person_id = $1 AND archived_at IS NULL`, personID)
	if err != nil {
		return false, fmt.Errorf("people: reading the subject's addresses: %w", err)
	}
	defer rows.Close()
	var addresses []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return false, fmt.Errorf("people: scanning a subject address: %w", err)
		}
		addresses = append(addresses, email)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("people: reading the subject's addresses: %w", err)
	}
	for _, email := range addresses {
		suppressed, err := storekit.EmailSuppressed(ctx, tx, email)
		if err != nil {
			return false, err
		}
		if suppressed {
			return true, nil
		}
	}
	return false, nil
}

// DuplicateCluster returns the other person records an open duplicate
// candidate ties to this one. It is what stops two records of the same human
// each buying the same answer (PI-PARAM-9); an empty answer degrades the
// fence to the single-record rule rather than blocking work.
//
// Both the candidate and the twin must be LIVE. A pair the queue has retired is
// not a claim about anybody any more, and a twin that has been archived — or
// anonymized in place by an erasure, which stamps archived_at and leaves the row
// — is not a record whose spend this one should be charged against. Without the
// second term this went on clustering a person the product has erased.
func DuplicateCluster(ctx context.Context, tx pgx.Tx, personID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT CASE WHEN d.left_person_id = $1 THEN d.right_person_id ELSE d.left_person_id END::text
		  FROM dedupe_candidate d
		  JOIN person twin
		    ON twin.id = CASE WHEN d.left_person_id = $1 THEN d.right_person_id ELSE d.left_person_id END
		 WHERE d.entity_type = 'person'
		   AND (d.left_person_id = $1 OR d.right_person_id = $1)
		   AND d.disposition = 'open'
		   AND d.archived_at IS NULL
		   AND twin.archived_at IS NULL`, personID)
	if err != nil {
		return nil, fmt.Errorf("people: reading the duplicate cluster: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("people: scanning a duplicate candidate: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading the duplicate cluster: %w", err)
	}
	return out, nil
}

// SubjectNameOnly resolves the name half of the same answer, for a caller that
// may read a person but not their employment edge. The employer is a
// relationship read and carries its own grant; a caller without it gets what
// it may see rather than a refusal, because the name alone is still a partial
// answer to "could a provider match this contact".
//
// Split out rather than copied so the full_name fallback below has one
// spelling: two would drift, and the drift a reader would see is one surface
// finding a contact by name and another not.
func SubjectNameOnly(ctx context.Context, tx pgx.Tx, personID string) (provider.PersonIdentifiers, error) {
	var id provider.PersonIdentifiers
	var first, last *string
	var full string
	if err := tx.QueryRow(ctx, `
		SELECT first_name, last_name, full_name FROM person WHERE id = $1`, personID).
		Scan(&first, &last, &full); err != nil {
		return provider.PersonIdentifiers{}, fmt.Errorf("people: reading the subject's name: %w", err)
	}
	// The stored parts FIRST. full_name is a display string that may hold a
	// title, a suffix or a company; splitting it when the real columns are
	// populated invents a worse answer than the one already on the record.
	id.FirstName, id.LastName = derefOr(first), derefOr(last)
	if id.FirstName == "" && id.LastName == "" {
		id.FirstName, id.LastName = splitFullName(full)
	}
	return id, nil
}

// SubjectIdentifiers resolves the closed set of facts that may leave the
// installation for one subject (PI-PARAM-11). Nothing else about them can:
// the provider port names these five fields and the adapter sends what it is
// given.
func SubjectIdentifiers(ctx context.Context, tx pgx.Tx, personID string) (provider.PersonIdentifiers, error) {
	id, err := SubjectNameOnly(ctx, tx, personID)
	if err != nil {
		return provider.PersonIdentifiers{}, err
	}

	// The employer, and its primary domain if one is on file. A domain
	// identifies a company to a provider far better than a name does, so it
	// is worth the join; its absence is not, which is why this is a LEFT
	// JOIN over the domain rather than a second failing query.
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(o.display_name, ''), coalesce(d.domain, '')
		  FROM relationship r
		  JOIN organization o ON o.id = r.organization_id
		  LEFT JOIN organization_domain d
		    ON d.organization_id = o.id AND d.is_primary AND d.archived_at IS NULL
		 WHERE r.kind = 'employment' AND r.person_id = $1
		   AND `+CurrentPrimaryEmploymentSQL("r")+` AND r.archived_at IS NULL
		 LIMIT 1`, personID).
		Scan(&id.CompanyName, &id.CompanyDomain); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return provider.PersonIdentifiers{}, fmt.Errorf("people: reading the subject's employer: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		SELECT handle FROM person_social
		 WHERE person_id = $1 AND platform = 'linkedin'
		 ORDER BY created_at LIMIT 1`, personID).Scan(&id.LinkedInURL); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return provider.PersonIdentifiers{}, fmt.Errorf("people: reading the subject's LinkedIn handle: %w", err)
	}
	return id, nil
}

// splitFullName is the fallback for a record whose name parts were never
// populated. One token is a first name — a mononym is a real name, and
// assigning it to the surname would send a request nobody matches. Everything
// after the first token is the surname, so multi-part family names ("van der
// Berg") stay whole rather than losing their particles.
func splitFullName(full string) (first, last string) {
	parts := strings.Fields(full)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
