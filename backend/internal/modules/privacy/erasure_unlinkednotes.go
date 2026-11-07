// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const erasedTextMention = "Erased"

func redactUnlinkedNotes(ctx context.Context, tx pgx.Tx, keys subjectKeys) error {
	patterns := addressPatterns(keys.emails)
	if keys.displayName != erasedName {
		if name, ok := values.FullNameTextPattern(keys.displayName, 1); ok {
			patterns = append(patterns, name)
		}
	}
	if len(patterns) == 0 {
		return nil
	}
	changed, err := scrubbedIDs(ctx, tx, `UPDATE activity a
 SET subject=regexp_replace(a.subject,@pattern,@erased,'gi'),body=regexp_replace(a.body,@pattern,@erased,'gi')
 WHERE a.kind='note' AND a.restricted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM activity_link l WHERE l.activity_id=a.id AND l.contact_id IS NOT NULL)
 AND NOT `+retentionscope.HeldThroughAnyLink("a.id")+`
 AND (a.subject IS DISTINCT FROM regexp_replace(a.subject,@pattern,@erased,'gi')
 OR a.body IS DISTINCT FROM regexp_replace(a.body,@pattern,@erased,'gi')) RETURNING a.id`,
		pgx.StrictNamedArgs{"pattern": strings.Join(patterns, "|"), "erased": erasedTextMention})
	if err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "activity", changed, subjectRedactionReason, subjectRedactionCause)
}
