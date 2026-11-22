// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type linkedInRepairRow struct {
	ID     ids.UUID
	Owner  ids.UUID
	Handle string
}

// NormalizeLeadLinkedInTx rewrites one bounded page of stored lead profiles.
func NormalizeLeadLinkedInTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, id, linkedin_url FROM lead
		 WHERE linkedin_url IS NOT NULL AND id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading lead LinkedIn profiles to normalize: %w", err)
	}
	return normalizeLinkedInRows(ctx, tx, rows, normalizeLeadLinkedIn)
}

// NormalizeContactLinkedInTx rewrites one bounded page of stored contact profiles.
func NormalizeContactLinkedInTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, contact_id, handle FROM contact_social
		 WHERE platform = $1 AND id > $2 ORDER BY id LIMIT $3`, SocialLinkedIn, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading contact LinkedIn profiles to normalize: %w", err)
	}
	return normalizeLinkedInRows(ctx, tx, rows, normalizeContactLinkedIn)
}

func normalizeLinkedInRows(
	ctx context.Context,
	tx pgx.Tx,
	rows pgx.Rows,
	write func(context.Context, pgx.Tx, linkedInRepairRow) (bool, error),
) ([]ids.UUID, int, error) {
	stored, err := pgx.CollectRows(rows, pgx.RowToStructByPos[linkedInRepairRow])
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: scanning LinkedIn profiles to normalize: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(stored))
	changed := 0
	for _, row := range stored {
		inspected = append(inspected, row.ID)
		wrote, err := write(ctx, tx, row)
		if err != nil {
			return nil, 0, err
		}
		if wrote {
			changed++
		}
	}
	return inspected, changed, nil
}

func normalizeLeadLinkedIn(ctx context.Context, tx pgx.Tx, row linkedInRepairRow) (bool, error) {
	normalized, err := NormalizeContactLinkedInURL(row.Handle)
	if err != nil {
		if _, invalid := errors.AsType[*values.ParseError](err); invalid {
			return false, nil
		}
		return false, fmt.Errorf("contacts: normalizing a stored lead LinkedIn profile: %w", err)
	}
	if normalized == row.Handle {
		return false, nil
	}
	if _, err := storekit.LockRow(ctx, tx, entityLead, row.Owner, storekit.IncludeArchived); err != nil {
		return false, err
	}
	if _, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{LinkedInURLs: []string{normalized}}); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE lead SET linkedin_url = $2 WHERE id = $1 AND linkedin_url = $3`, row.Owner, normalized, row.Handle)
	if err != nil {
		return false, fmt.Errorf("contacts: normalizing a lead LinkedIn profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityLead, row.Owner,
		map[string]any{linkedinURLField: row.Handle}, map[string]any{linkedinURLField: normalized})
	if err != nil {
		return false, err
	}
	err = storekit.EmitEvent(ctx, tx, auditID, row.Owner, crmcontracts.PublicEventLeadUpdated{
		ChangedFields: map[string]any{linkedinURLField: normalized},
	})
	return err == nil, err
}

func normalizeContactLinkedIn(ctx context.Context, tx pgx.Tx, row linkedInRepairRow) (bool, error) {
	normalized, err := NormalizeContactLinkedInURL(row.Handle)
	if err != nil {
		if _, invalid := errors.AsType[*values.ParseError](err); invalid {
			return false, nil
		}
		return false, fmt.Errorf("contacts: normalizing a stored contact LinkedIn profile: %w", err)
	}
	if normalized == row.Handle {
		return false, nil
	}
	if _, err := storekit.LockRow(ctx, tx, contactEntity, row.Owner, storekit.IncludeArchived); err != nil {
		return false, err
	}
	if _, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{LinkedInURLs: []string{normalized}}); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE contact_social SET handle = $2 WHERE id = $1 AND handle = $3`, row.ID, normalized, row.Handle)
	if err != nil {
		return false, fmt.Errorf("contacts: normalizing a contact LinkedIn profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE contact SET updated_at = $2 WHERE id = $1`, row.Owner, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("contacts: marking a normalized contact LinkedIn profile: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", contactEntity, row.Owner,
		map[string]any{fieldSocialLinkedIn: row.Handle}, map[string]any{fieldSocialLinkedIn: normalized})
	if err != nil {
		return false, err
	}
	err = storekit.EmitEvent(ctx, tx, auditID, row.Owner, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldSocialLinkedIn: normalized},
	})
	return err == nil, err
}

// CountStoredLinkedIn returns a bounded count for one normalization step.
func CountStoredLinkedIn(ctx context.Context, q datatask.Querier, table string, maximum int64) (int64, error) {
	var query string
	switch table {
	case entityLead:
		query = `SELECT 1 FROM lead WHERE linkedin_url IS NOT NULL LIMIT $1`
	case contactEntity:
		query = `SELECT 1 FROM contact_social WHERE platform = 'linkedin' LIMIT $1`
	default:
		return 0, fmt.Errorf("contacts: unsupported LinkedIn repair table %q", table)
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (`+query+`) stored`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("contacts: counting stored LinkedIn profiles: %w", err)
	}
	return count, nil
}

type leadContactRepairPage struct {
	Inspected []ids.UUID
	Merged    int
	Ambiguous []ids.LeadID
}

// leadContactCleanupSource names the lead and contact cleanup on the queue rows it files.
const leadContactCleanupSource = "lead_contact_cleanup"

type leadContactPair struct {
	LeadID     ids.LeadID
	ContactIDs []ids.UUID
	Fields     []string
	Values     []string
	Email      *string
	LinkedIn   *string
}

// leadPairColumns is what one lead's pair read returns, in leadContactPair order.
const leadPairColumns = `l.id, matches.contact_ids, matches.fields, matches.match_values, l.email, l.linkedin_url `

// maxContactsPerLeadReview bounds the pairs one lead files in one transaction.
// A lead matching more contacts stays unmerged and in the cleanup's scope.
const maxContactsPerLeadReview = 10

// existingLeadContactPairs lists live leads that share an identity with a live
// contact. It skips a lead whose every pair of contacts a person already
// dismissed as not a duplicate, so that lead stays a lead for good. Fields and
// Values name, per contact in id order, the identity that matched.
const existingLeadContactPairs = `
	FROM lead l
	CROSS JOIN LATERAL (
		SELECT array_agg(by_contact.contact_id ORDER BY by_contact.contact_id) AS contact_ids,
		       array_agg(by_contact.field ORDER BY by_contact.contact_id) AS fields,
		       array_agg(by_contact.value ORDER BY by_contact.contact_id) AS match_values
		FROM (
			SELECT DISTINCT ON (candidate.contact_id) candidate.contact_id, candidate.field, candidate.value
			FROM (
				SELECT ce.contact_id, 'email' AS field, ce.email AS value
				  FROM contact_email ce JOIN contact c ON c.id = ce.contact_id
				 WHERE ce.archived_at IS NULL AND c.archived_at IS NULL AND ce.email = l.email
				UNION ALL
				SELECT cs.contact_id, 'linkedin', cs.handle
				  FROM contact_social cs JOIN contact c ON c.id = cs.contact_id
				 WHERE c.archived_at IS NULL AND cs.platform = 'linkedin' AND cs.handle = l.linkedin_url
				UNION ALL
				SELECT c.id, 'matched_lane', 'linked_lead' FROM contact c WHERE c.id = l.from_contact_id AND c.archived_at IS NULL
				UNION ALL
				SELECT c.id, 'matched_lane', 'linked_lead' FROM contact c WHERE c.converted_from_lead_id = l.id AND c.archived_at IS NULL
			) candidate
			ORDER BY candidate.contact_id, candidate.field
		) by_contact
	) matches
	WHERE l.archived_at IS NULL AND cardinality(matches.contact_ids) > 0
	  AND NOT (cardinality(matches.contact_ids) > 1 AND NOT EXISTS (
		SELECT 1 FROM unnest(matches.contact_ids) AS a(id)
		  JOIN unnest(matches.contact_ids) AS b(id) ON a.id < b.id
		 WHERE NOT EXISTS (
			SELECT 1 FROM dedupe_candidate d
			 WHERE d.entity_type = 'contact' AND d.disposition = 'not_a_duplicate'
			   AND d.left_contact_id = a.id AND d.right_contact_id = b.id)))`

// MergeExistingLeadContactPairsTx promotes unambiguous pairs into the contact.
// A lead that matches several contacts is left alone. Each pair of those
// contacts goes to the duplicate review. Merging them makes the lead
// unambiguous for the next run.
func (s *Store) MergeExistingLeadContactPairsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (leadContactRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return leadContactRepairPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT `+leadPairColumns+existingLeadContactPairs+`
		 AND l.id > $1 ORDER BY l.id LIMIT $2`, after, limit)
	if err != nil {
		return leadContactRepairPage{}, fmt.Errorf("contacts: reading lead and contact pairs: %w", err)
	}
	pairs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[leadContactPair])
	if err != nil {
		return leadContactRepairPage{}, fmt.Errorf("contacts: scanning lead and contact pairs: %w", err)
	}
	page := leadContactRepairPage{Inspected: make([]ids.UUID, 0, len(pairs))}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return leadContactRepairPage{}, err
	}
	for _, pair := range pairs {
		page.Inspected = append(page.Inspected, pair.LeadID.UUID)
		if len(pair.ContactIDs) != 1 {
			if err := pair.fileContactPairs(ctx, tx, by); err != nil {
				return leadContactRepairPage{}, err
			}
			page.Ambiguous = append(page.Ambiguous, pair.LeadID)
			continue
		}
		contactID := ids.From[ids.ContactKind](pair.ContactIDs[0])
		if err := s.mergeExistingLeadContactPairTx(ctx, tx, pair.LeadID, contactID, by); err != nil {
			return leadContactRepairPage{}, err
		}
		page.Merged++
	}
	return page, nil
}

// fileContactPairs puts every pair of the contacts one lead matches on the
// duplicate review. A pair the queue already holds, open or dismissed, stays as it is.
// A lead over the cap, or whose matches changed since the page was read, files nothing.
func (p leadContactPair) fileContactPairs(ctx context.Context, tx pgx.Tx, by string) error {
	if len(p.ContactIDs) > maxContactsPerLeadReview {
		return nil
	}
	fresh, current, err := p.rereadUnderLocks(ctx, tx)
	if err != nil || !current {
		return err
	}
	for i := range fresh.ContactIDs {
		for j := i + 1; j < len(fresh.ContactIDs); j++ {
			_, err := recordDedupeCandidate(ctx, tx, entityContact, fresh.ContactIDs[i], fresh.ContactIDs[j], 0,
				fresh.evidence(i, j), leadContactCleanupSource, by)
			if err != nil {
				return fmt.Errorf("contacts: filing the contacts one lead matches: %w", err)
			}
		}
	}
	return nil
}

// rereadUnderLocks locks the matched contacts in id order, then the lead's
// identities. A contact edit takes them in the same order. It then reads the
// lead's matches again. It answers false when they are no longer the contacts
// the page saw, so a concurrent edit is left for the next run.
func (p leadContactPair) rereadUnderLocks(ctx context.Context, tx pgx.Tx) (leadContactPair, bool, error) {
	for _, contact := range p.ContactIDs {
		if err := auth.EnsureWritableLive(ctx, tx, contactEntity, contact); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return leadContactPair{}, false, nil
			}
			return leadContactPair{}, false, err
		}
		if _, err := storekit.LockRow(ctx, tx, contactEntity, contact, storekit.LiveOnly); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return leadContactPair{}, false, nil
			}
			return leadContactPair{}, false, err
		}
	}
	keys := storekit.IdentityKeys{}
	if p.Email != nil {
		keys.Emails = []string{*p.Email}
	}
	if p.LinkedIn != nil {
		keys.LinkedInURLs = []string{*p.LinkedIn}
	}
	if _, err := storekit.LockIdentityHolders(ctx, tx, keys); err != nil {
		return leadContactPair{}, false, err
	}
	rows, err := tx.Query(ctx, `SELECT `+leadPairColumns+existingLeadContactPairs+` AND l.id = $1`, p.LeadID)
	if err != nil {
		return leadContactPair{}, false, fmt.Errorf("contacts: rereading a lead's contacts: %w", err)
	}
	fresh, err := pgx.CollectRows(rows, pgx.RowToStructByPos[leadContactPair])
	if err != nil {
		return leadContactPair{}, false, fmt.Errorf("contacts: scanning a lead's contacts: %w", err)
	}
	if len(fresh) != 1 || !slices.Equal(fresh[0].ContactIDs, p.ContactIDs) {
		return leadContactPair{}, false, nil
	}
	return fresh[0], true, nil
}

// evidence shows the identity both contacts matched the lead on, or the
// generic lane when they matched in different ways.
func (p leadContactPair) evidence(i, j int) []map[string]any {
	field := p.Fields[i]
	if p.Fields[j] != field {
		field = "matched_lane"
	}
	return []map[string]any{{
		evidenceFieldKey: field, evidenceLeftKey: p.Values[i], evidenceRightKey: p.Values[j],
		evidenceSignalKey: evidenceSignalCollide,
	}}
}

// MergeExistingLeadContactPairForEmailTx applies the cleanup task's merge to
// the one unambiguous historical pair named by an accepted sender.
func (s *Store) MergeExistingLeadContactPairForEmailTx(ctx context.Context, tx pgx.Tx, email string, contact ids.ContactID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	holders, err := storekit.IdentityHoldersOf(ctx, tx, storekit.IdentityKeys{Emails: []string{email}})
	if err != nil {
		return err
	}
	if len(holders.Leads) != 1 || len(holders.Contacts) != 1 || holders.Contacts[0] != contact {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.mergeExistingLeadContactPairTx(ctx, tx, holders.Leads[0], contact, by)
}

func (s *Store) mergeExistingLeadContactPairTx(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID, by string) error {
	out, err := s.promoteLeadTx(ctx, tx, lead, PromoteLeadInput{
		Trigger: string(TriggerHumanQualify), intoCandidate: true,
	}, by, nil, contact)
	if err != nil {
		return err
	}
	if !out.Merged || ids.UUID(out.Contact.Id) != contact.UUID {
		return errors.New("contacts: existing lead and contact pair did not merge into its contact")
	}
	return nil
}

// CountExistingLeadContactPairs returns a bounded estimate of live pairs.
func CountExistingLeadContactPairs(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT l.id `+existingLeadContactPairs+` LIMIT $1) pairs`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting lead and contact pairs: %w", err)
	}
	return count, nil
}
