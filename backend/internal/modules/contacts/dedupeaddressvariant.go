// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The address-variant lane. Two contacts whose addresses differ only by a plus
// tag (`ab@` and `ab+accounting@`) or a Gmail spelling are likely one owner.
// The exact email lane compares whole addresses, and the fuzzy tier needs a
// name that a tagged address usually does not carry.
//
// It is a review lane and never an exact key, because a tag can be shared. A
// team alias such as `sales+partner@` reaches several humans, and routing on
// the base address would put one human's mail on another's timeline. So the
// lane runs only for callers that opt in (ContactCandidate.QueueNameCollisions).

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addressBaseSQL is the SQL side of correspondence.BaseAddress. The anchored
// pattern admits one `@` only, as the Go cut at the first `@` does. The
// migration that builds idx_contact_email_mailbox_base spells the same
// expression, and TestTheAddressVariantLaneReadsTheMailboxBaseIndex holds it.
func addressBaseSQL(expr string) string {
	untagged := storekit.SQLf(`regexp_replace(btrim(lower(%s)), '^([^+@]+)\+[^@]*@', '\1@')`, expr)
	return storekit.SQLf(
		`CASE WHEN %[1]s ~ '^[^@]*@(gmail|googlemail)\.com$' THEN replace(regexp_replace(%[1]s, '@(gmail|googlemail)\.com$', ''), '.', '') || '@gmail.com' ELSE %[1]s END`,
		untagged)
}

// addressBases is the candidate's addresses in the two forms the lane compares.
// The bases are sorted and distinct, so lockAddressBaseLane takes them in one
// fixed order. The folded addresses are kept as written.
func addressBases(emails []string) (bases, folded []string) {
	for _, e := range emails {
		f := correspondence.Fold(e)
		if f == "" {
			continue
		}
		base, _ := correspondence.BaseAddress(f)
		bases = append(bases, base)
		folded = append(folded, f)
	}
	slices.Sort(bases)
	return slices.Compact(bases), folded
}

// lockAddressBaseLane serializes the lane's read with the address write that
// follows it, for the reason lockPhoneLane gives. No unique index can hold a
// base address. Without the lock, two creates of `ab@` and `ab+x@` both read
// no incumbent and commit with no pair. The fixed lock order is phone, then
// address base, then name. A LinkedIn handle write takes its profile lock
// after these (fileLinkedInTwinsAtWrite).
func lockAddressBaseLane(ctx context.Context, tx pgx.Tx, emails []string) error {
	bases, _ := addressBases(emails)
	for _, base := range bases {
		if err := storekit.LockWriteIdentity(ctx, tx, "contact_email_base", base); err != nil {
			return err
		}
	}
	return nil
}

// addressVariantQuery takes the bases, the folded addresses, the contact to
// leave out and the row limit. Holders the queue already pairs with that
// contact are dropped before the limit, as newRivals drops them. Otherwise
// many contacts on one Gmail base hide the rest.
var addressVariantQuery = `
		SELECT e.contact_id, array_agg(e.email)
		  FROM contact_email e
		  JOIN contact p ON p.id = e.contact_id AND p.archived_at IS NULL
		 WHERE ` + addressBaseSQL("e.email") + ` = ANY($1)
		   AND lower(e.email) <> ALL($2)
		   AND e.archived_at IS NULL
		   AND ($3::uuid IS NULL OR (e.contact_id <> $3 AND NOT EXISTS (
		         SELECT 1 FROM dedupe_candidate d
		          WHERE ` + heldPairSQL("'"+entityContact+"'", "$3", "e.contact_id") + `)))
		 GROUP BY e.contact_id
		 ORDER BY e.contact_id
		 LIMIT $4`

// addressVariantContacts finds every live contact holding an address with the
// same base as one of the candidate's and a different full address. They come
// lowest contact id first. Each one is a pair of its own. Three contacts on one
// base are three pairs, and a hidden holder must not mask a visible one.
func addressVariantContacts(ctx context.Context, tx pgx.Tx, c ContactCandidate) ([]ReviewMatch, error) {
	bases, folded := addressBases(c.Emails)
	if len(bases) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, addressVariantQuery, bases, folded, c.ExcludeID, dedupeRivalLimit)
	if err != nil {
		return nil, fmt.Errorf("dedupe contact address variant: %w", err)
	}
	type holder struct {
		id   ids.ContactID
		held []string
	}
	holders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (holder, error) {
		var h holder
		err := row.Scan(&h.id, &h.held)
		return h, err
	})
	if err != nil {
		return nil, fmt.Errorf("dedupe contact address variant: %w", err)
	}
	matches := make([]ReviewMatch, 0, len(holders))
	for _, h := range holders {
		candidate, incumbent, ok := addressVariantPair(folded, h.held)
		if !ok {
			return nil, fmt.Errorf("dedupe contact address variant: %s holds no address the lane matched", h.id)
		}
		matches = append(matches, ReviewMatch{
			Decision: DecisionAddressVariantReview, ContactID: h.id,
			CandidateEmail: candidate, IncumbentEmail: incumbent,
		})
	}
	return matches, nil
}

// addressVariantPair is the address-variant lane's pair test: an address of
// each side with one base and two different full addresses. It answers the
// lowest such incumbent address, so the evidence is the same on every run.
func addressVariantPair(candidate, incumbent []string) (string, string, bool) {
	held := slices.Clone(incumbent)
	slices.Sort(held)
	for _, address := range held {
		folded := correspondence.Fold(address)
		base, _ := correspondence.BaseAddress(folded)
		for _, c := range candidate {
			f := correspondence.Fold(c)
			if candidateBase, _ := correspondence.BaseAddress(f); candidateBase == base && f != folded {
				return f, folded, true
			}
		}
	}
	return "", "", false
}

// addressVariantEvidence is what the review queue shows for the pair: the two
// addresses side by side. The lane compared strings and scored nothing, so the
// evidence carries no score and the stored confidence is 0.
func addressVariantEvidence(candidate, incumbent string) []map[string]any {
	return []map[string]any{{
		evidenceFieldKey:  fieldEmail,
		evidenceLeftKey:   candidate,
		evidenceRightKey:  incumbent,
		evidenceSignalKey: evidenceSignalCollide,
	}}
}
