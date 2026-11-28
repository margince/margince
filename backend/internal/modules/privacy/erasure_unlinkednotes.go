// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	erasedTextMention    = "Erased"
	textPatternParameter = "pattern"
	erasedTextParameter  = "erased"
)

// subjectTextPattern matches the subject's addresses and full name inside free
// text, or is empty when the subject has neither to match.
func subjectTextPattern(emails []string, displayName string) string {
	patterns := addressPatterns(emails)
	if displayName != erasedName {
		if name, ok := values.FullNameTextPattern(displayName, 2); ok {
			patterns = append(patterns, name)
		}
	}
	return strings.Join(patterns, "|")
}

func redactUnlinkedNotes(ctx context.Context, tx pgx.Tx, keys subjectKeys) ([]ids.UUID, error) {
	pattern := subjectTextPattern(keys.emails, keys.displayName)
	if pattern == "" {
		return nil, nil
	}
	locked, err := lockNoteMentions(ctx, tx, pattern)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `UPDATE activity a
 SET subject=CASE WHEN a.kind='note' THEN regexp_replace(a.subject,@pattern,@erased,'gi') ELSE a.subject END,
 body=CASE WHEN a.kind='note' THEN regexp_replace(a.body,@pattern,@erased,'gi') ELSE a.body END,
 meeting_notes=regexp_replace(a.meeting_notes,@pattern,@erased,'gi'),
 body_own_end=CASE WHEN a.kind='note' THEN NULL ELSE a.body_own_end END,
 raw=CASE WHEN a.kind='note' THEN NULL ELSE a.raw END,
 source_author_id=CASE WHEN a.kind='note' THEN NULL ELSE a.source_author_id END,
 source_author_name=CASE WHEN a.kind='note' THEN NULL ELSE a.source_author_name END
 WHERE a.id=ANY(@locked::uuid[]) AND a.kind IN ('note','meeting') AND a.restricted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM activity_link l WHERE l.activity_id=a.id AND (l.contact_id IS NOT NULL OR l.lead_id IS NOT NULL))
 AND NOT `+retentionscope.HeldThroughAnyLink("a.id")+`
 AND ((a.kind='note' AND (a.subject IS DISTINCT FROM regexp_replace(a.subject,@pattern,@erased,'gi')
 OR a.body IS DISTINCT FROM regexp_replace(a.body,@pattern,@erased,'gi')))
 OR a.meeting_notes IS DISTINCT FROM regexp_replace(a.meeting_notes,@pattern,@erased,'gi')) RETURNING a.id,a.kind`,
		pgx.StrictNamedArgs{textPatternParameter: pattern, erasedTextParameter: erasedTextMention, "locked": locked})
	if err != nil {
		return nil, err
	}
	changed, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct {
		ID   ids.UUID
		Kind string
	}])
	if err != nil {
		return nil, err
	}
	idsChanged := make([]ids.UUID, 0, len(changed))
	for _, row := range changed {
		id := row.ID
		idsChanged = append(idsChanged, id)

		proof, err := certifyCollateralScrub(ctx, tx, "activity", id, subjectRedactionReason, subjectRedactionCause)
		if err != nil {
			return nil, err
		}
		fields := crmcontracts.PublicEventActivityChangedFields{}
		if row.Kind == "meeting" {
			fields.MeetingNotes = new(true)
		} else {
			fields.Body = new(true)
		}
		if err := storekit.EmitEvent(ctx, tx, proof, id, crmcontracts.PublicEventActivityUpdated{ChangedFields: fields}); err != nil {
			return nil, err
		}
	}
	return idsChanged, nil
}
