// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The LinkedIn lane. Two live contacts holding one LinkedIn profile are likely
// one human, whatever their addresses and names say. It is a review lane and
// never a key, because a pasted profile can be the wrong one. The write stands
// and the pair goes on the review queue.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkedInLaneSource names this detector on the queue rows it files.
const linkedInLaneSource = "linkedin_profile"

// linkedInProfileKey is the profile a stored LinkedIn handle names: its slug
// under /in/, lower case. A localized host, a trailing slash, a query or a
// deeper path names the same profile. A company or school page names none.
//
// It matches more loosely than the lead and contact identity key, which
// compares the stored URL as written. That key routes a record and may not guess.
// This lane only asks a human, so a near spelling is worth the question.
func linkedInProfileKey(handle string) (string, bool) {
	normalized, err := NormalizeLinkedInURL(handle)
	if err != nil || !isLinkedinURL(normalized) {
		return "", false
	}
	slug, ok := linkedInProfileSlug(normalized)
	return strings.ToLower(slug), ok
}

// linkedInProfileKeySQL is linkedInProfileKey over a stored handle, NULL when
// the handle names no profile. idx_contact_social_linkedin_profile indexes the
// same expression. TestTheLinkedInProfileKeyAgreesInGoAndSQL holds the two to
// one answer.
func linkedInProfileKeySQL(expr string) string {
	return storekit.SQLf(
		`substring(lower(%s) from '^(?:https?://)?(?:[a-z0-9-]+\.)*linkedin\.com/in/([^/?#]+)')`, expr)
}

// fileLinkedInTwinsAtWrite files the pairs of a LinkedIn handle a write just
// set. It runs where a handle lands, so a create, an edit, an import and a
// provider fill file the same pairs.
func fileLinkedInTwinsAtWrite(ctx context.Context, tx pgx.Tx, contact ids.ContactID, handle string) error {
	key, ok := linkedInProfileKey(handle)
	if !ok {
		return nil
	}
	// No unique index can hold a profile key, because the lane must not refuse
	// the write. Without the lock, two writes of one profile both read no twin.
	// It comes after the lead_linkedin and address locks the writers take.
	if err := storekit.LockWriteIdentity(ctx, tx, "contact_linkedin_profile", key); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	_, err = fileLinkedInTwins(ctx, tx, contact, handle, linkedInLaneSource, by)
	return err
}

// fileLinkedInTwins puts the contact holding handle and every other live
// holder of the same profile on the review queue. It answers how many pairs
// are new. It takes no lock, because the duplicate scan reaches it through
// fileDuplicates, which takes none.
func fileLinkedInTwins(ctx context.Context, tx pgx.Tx, contact ids.ContactID, handle, source, by string) (int, error) {
	key, ok := linkedInProfileKey(handle)
	if !ok {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT s.contact_id, s.handle
		  FROM contact_social s
		  JOIN contact c ON c.id = s.contact_id AND c.archived_at IS NULL
		 WHERE s.platform = '`+SocialLinkedIn+`' AND `+linkedInProfileKeySQL("s.handle")+` = $1
		   AND s.contact_id <> $2
		 ORDER BY s.contact_id`, key, contact)
	if err != nil {
		return 0, fmt.Errorf("dedupe contact linkedin: %w", err)
	}
	type holder struct {
		id     ids.ContactID
		handle string
	}
	holders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (holder, error) {
		var h holder
		err := row.Scan(&h.id, &h.handle)
		return h, err
	})
	if err != nil {
		return 0, fmt.Errorf("dedupe contact linkedin: %w", err)
	}
	// The cap comes after the pairs the queue already holds. A profile held by
	// more than dedupeRivalLimit contacts reaches the later ones on a later pass.
	holders, err = newRivals(ctx, tx, entityContact, &contact.UUID, holders,
		func(h holder) ids.UUID { return h.id.UUID })
	if err != nil {
		return 0, err
	}
	filed := 0
	for _, h := range holders {
		recorded, err := recordDedupeCandidate(ctx, tx, entityContact, contact.UUID, h.id.UUID, 0,
			linkedInEvidence(handle, h.handle), source, by)
		if err != nil {
			return filed, fmt.Errorf("record contact linkedin candidate: %w", err)
		}
		if recorded {
			filed++
		}
	}
	return filed, nil
}

// linkedInProfilesMatch is the lane's pair test for the nightly re-check.
func linkedInProfilesMatch(a, b string) bool {
	left, ok := linkedInProfileKey(a)
	if !ok {
		return false
	}
	right, ok := linkedInProfileKey(b)
	return ok && left == right
}

// linkedInEvidence is what the review queue shows for the pair: the two
// profile URLs side by side. The lane compared keys and scored nothing.
func linkedInEvidence(candidate, incumbent string) []map[string]any {
	return []map[string]any{{
		evidenceFieldKey:  fieldLinkedin,
		evidenceLeftKey:   candidate,
		evidenceRightKey:  incumbent,
		evidenceSignalKey: evidenceSignalCollide,
	}}
}
