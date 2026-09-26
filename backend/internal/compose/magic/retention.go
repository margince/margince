// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What retention did, as counts.
//
// The done lane leaves out every audit row of a scrubbed record, so a receipt
// cannot republish what an erasure destroyed (privacy.UnscrubbedImageSQL). That
// boundary also hid the one machine action that destroys data: on a rehearsal
// install retention anonymized 1,200 leads and this page never said so.
//
// Retention is reported here as one line per policy and action, with a count
// and no record — no name, no id — so the boundary holds and the action is
// still visible. Only a reader who may read the retention policies sees it,
// because a count of anonymized records is a fact about the installation.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retentionSentences names what a retention action did to a kind of record. A
// pair this build has no sentence for is not shown: raw_capture and
// ai_call_payload are the engine's own copies, not records a reader keeps.
var retentionSentences = map[string]string{
	"lead/anonymize":    "magic.action.retention_lead_anonymize",
	"lead/archive":      "magic.action.retention_lead_archive",
	"contact/anonymize": "magic.action.retention_contact_anonymize",
	"contact/erase":     "magic.action.retention_contact_erase",
	"activity/archive":  "magic.action.retention_activity_archive",
	"activity/erase":    "magic.action.retention_activity_erase",
	"deal/archive":      "magic.action.retention_deal_archive",
}

// retentionSince reads what retention did in the window, one line per record
// kind, action and window, newest first.
func retentionSince(ctx context.Context, tx pgx.Tx, since time.Time) ([]crmcontracts.MagicLine, error) {
	if err := auth.Require(ctx, "retention_policy", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT a.entity_type, coalesce(a.evidence->>'retention_action', a.action),
		       coalesce(a.evidence->>'retain_days', ''),
		       count(DISTINCT a.entity_id), max(a.occurred_at),
		       (array_agg(a.id ORDER BY a.occurred_at DESC, a.id DESC))[1]
		  FROM audit_log a
		 WHERE a.occurred_at >= $1
		   -- Every retention action but one states itself in its evidence. A
		   -- contact erasure goes through the Art. 17 eraser, whose tombstone
		   -- records the reason instead (privacy/erasure_tombstone.go).
		   AND (a.evidence ? 'retention_action'
		        OR (a.action = 'erase' AND a.entity_type = 'contact' AND a.evidence->>'reason' = 'retention'))
		 GROUP BY 1, 2, 3
		 ORDER BY 5 DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("read what retention did: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.MagicLine
	for rows.Next() {
		var entityType, action, days string
		var count int
		var at time.Time
		var newest ids.UUID
		if err := rows.Scan(&entityType, &action, &days, &count, &at, &newest); err != nil {
			return nil, fmt.Errorf("read what retention did: %w", err)
		}
		key, ok := retentionSentences[entityType+"/"+action]
		if !ok {
			continue
		}
		out = append(out, retentionLine(newest, at, key, days, count))
	}
	return out, rows.Err()
}

func retentionLine(id ids.UUID, at time.Time, key, days string, count int) crmcontracts.MagicLine {
	line := crmcontracts.MagicLine{
		Id:         openapi_types.UUID(id),
		OccurredAt: at,
		Lane:       crmcontracts.MagicLineLaneMagicLaneDone,
		Summary:    crmcontracts.MagicSentence{Key: key},
		Actor: crmcontracts.MagicActor{
			Type:  crmcontracts.MagicActorTypeMagicActorSystem,
			Id:    "system",
			Label: &crmcontracts.MagicSentence{Key: "magic.by.retention"},
		},
		Count: &count,
	}
	if _, err := strconv.Atoi(days); err == nil {
		values := map[string]string{"days": days}
		line.Reason = &crmcontracts.MagicSentence{Key: "magic.why.retention", Values: &values}
	}
	return line
}
