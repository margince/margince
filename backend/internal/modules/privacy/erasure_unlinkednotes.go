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

const erasedTextMention = "Erased"

func redactUnlinkedNotes(ctx context.Context, tx pgx.Tx, keys subjectKeys) ([]ids.UUID, error) {
	patterns := addressPatterns(keys.emails)
	if keys.displayName != erasedName {
		if name, ok := values.FullNameTextPattern(keys.displayName, 2); ok {
			patterns = append(patterns, name)
		}
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	pattern := strings.Join(patterns, "|")
	locked, err := lockNoteMentions(ctx, tx, pattern)
	if err != nil {
		return nil, err
	}
	changed, err := scrubbedIDs(ctx, tx, `UPDATE activity a
 SET subject=regexp_replace(a.subject,@pattern,@erased,'gi'),body=regexp_replace(a.body,@pattern,@erased,'gi'),
 raw=NULL,source_author_id=NULL,source_author_name=NULL,version=a.version+1,updated_at=now()
 WHERE a.id=ANY(@locked::uuid[]) AND a.kind='note' AND a.restricted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM activity_link l WHERE l.activity_id=a.id AND (l.contact_id IS NOT NULL OR l.lead_id IS NOT NULL))
 AND NOT `+retentionscope.HeldThroughAnyLink("a.id")+`
 AND (a.subject IS DISTINCT FROM regexp_replace(a.subject,@pattern,@erased,'gi')
 OR a.body IS DISTINCT FROM regexp_replace(a.body,@pattern,@erased,'gi')) RETURNING a.id`,
		pgx.StrictNamedArgs{"pattern": pattern, "erased": erasedTextMention, "locked": locked})
	if err != nil {
		return nil, err
	}
	for _, id := range changed {
		proof, err := certifyCollateralScrub(ctx, tx, "activity", id, subjectRedactionReason, subjectRedactionCause)
		if err != nil {
			return nil, err
		}
		if err := storekit.EmitEvent(ctx, tx, proof, id, crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: new(true)}}); err != nil {
			return nil, err
		}
	}
	return changed, nil
}
