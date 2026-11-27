// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeSubjectBriefCache destroys every reader's cached relationship brief for
// a subject the caller has just anonymized. The brief holds what the model was
// shown: claims from the subject's conversations and a line on each message.
//
// The erasure leaves the contact row in place, so no ON DELETE CASCADE fires.
// The row is deleted and not emptied, because the next read rebuilds it.
func purgeSubjectBriefCache(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	if err := generatedsummary.ForgetContacts(ctx, tx, subject); err != nil {
		return fmt.Errorf("privacy: destroying the subject's cached relationship brief: %w", err)
	}
	return purgeSummariesWrittenFrom(ctx, tx, subject, nil)
}

// summarisedRecords selects the companies and deals whose summaries were
// written with the subject in view. $1 is the contact and $2 the messages found
// by address alone, which carry no link to them.
//
// A deal's company is included because the account brief folds its deals in.
const summarisedRecords = `
	WITH spoken AS (
	    SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1
	    UNION
	    SELECT unnest($2::uuid[])
	), record AS (
	    SELECT l.company_id, l.deal_id FROM activity_link l
	     WHERE l.activity_id IN (SELECT activity_id FROM spoken)
	    UNION
	    SELECT r.company_id, r.deal_id FROM relationship r WHERE r.contact_id = $1
	), account AS (
	    SELECT company_id, deal_id FROM record
	    UNION
	    SELECT d.company_id, NULL FROM deal d WHERE d.id IN (SELECT deal_id FROM record)
	)
	SELECT coalesce(array_agg(DISTINCT company_id) FILTER (WHERE company_id IS NOT NULL), '{}'),
	       coalesce(array_agg(DISTINCT deal_id) FILTER (WHERE deal_id IS NOT NULL), '{}')
	  FROM account`

// purgeSummariesWrittenFrom destroys every reader's cached summaries of the
// companies and deals the subject spoke on. A deal's status card and an account
// brief quote the timeline, and each is keyed by its record, not by the contact.
func purgeSummariesWrittenFrom(ctx context.Context, tx pgx.Tx, subject ids.ContactID, activities []ids.UUID) error {
	var companies, deals []ids.UUID
	if err := tx.QueryRow(ctx, summarisedRecords, subject, activities).Scan(&companies, &deals); err != nil {
		return fmt.Errorf("privacy: finding the summaries written from the subject's words: %w", err)
	}
	if err := generatedsummary.ForgetRecords(ctx, tx, companies, deals); err != nil {
		return fmt.Errorf("privacy: destroying the summaries written from the subject's words: %w", err)
	}
	return nil
}
