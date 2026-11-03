// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
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

type leadContactPair struct {
	LeadID     ids.LeadID
	ContactIDs []ids.UUID
}

const existingLeadContactPairs = `
	FROM lead l
	CROSS JOIN LATERAL (
		SELECT array_agg(candidate.contact_id ORDER BY candidate.contact_id) AS contact_ids
		FROM (
			SELECT ce.contact_id
			  FROM contact_email ce JOIN contact c ON c.id = ce.contact_id
			 WHERE ce.archived_at IS NULL AND c.archived_at IS NULL AND ce.email = l.email
			UNION
			SELECT cs.contact_id
			  FROM contact_social cs JOIN contact c ON c.id = cs.contact_id
			 WHERE c.archived_at IS NULL AND cs.platform = 'linkedin' AND cs.handle = l.linkedin_url
			UNION
			SELECT c.id FROM contact c WHERE c.id = l.from_contact_id AND c.archived_at IS NULL
			UNION
			SELECT c.id FROM contact c WHERE c.converted_from_lead_id = l.id AND c.archived_at IS NULL
		) candidate
	) matches
	WHERE l.archived_at IS NULL AND cardinality(matches.contact_ids) > 0`

// MergeExistingLeadContactPairsTx promotes unambiguous pairs into the contact.
func (s *Store) MergeExistingLeadContactPairsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (leadContactRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return leadContactRepairPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id, matches.contact_ids `+existingLeadContactPairs+`
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
