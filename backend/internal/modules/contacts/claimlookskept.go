// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A promise to invite someone looks kept when that someone accepted a later
// meeting. This is a hint on the claim and writes nothing: the reader settles
// it through the settle verb, with the audit, task and offer effects a human
// settle has.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// withLooksKept sets `looks_kept` on the open promises of ours in claims that a
// meeting this reader may open shows kept. The claims were read under the
// reader's scope already, so their sources are readable. The meeting and the
// guest are read under the same scope.
//
// The guest matches by full name on word boundaries, each part at least three
// letters, and must work at the same company as the claim's contact. The
// claim's own contact must be on the meeting. The meeting must start after the
// promise, with the guest's answer accepted, and must be neither archived nor
// called off.
func withLooksKept(ctx context.Context, tx pgx.Tx, claims []crmcontracts.ConversationClaim) error {
	var open []ids.UUID
	at := map[ids.UUID]int{}
	for i, claim := range claims {
		if claim.Kind == crmcontracts.ConversationClaimKindCommitmentOurs && claim.Status == crmcontracts.ConversationClaimStatusOpen {
			open = append(open, ids.UUID(claim.Id))
			at[ids.UUID(claim.Id)] = i
		}
	}
	if len(open) == 0 || !auth.ReadGranted(ctx, "activity") || !auth.ReadGranted(ctx, "contact") || !auth.ReadGranted(ctx, "relationship") {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimPos := arg(open)
	meetingScope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return err
	}
	if meetingScope == "" {
		meetingScope = sqlAlwaysVisible
	}
	guestScope, err := auth.ScopeClauseFor(ctx, "contact", "g", arg)
	if err != nil {
		return err
	}
	if guestScope == "" {
		guestScope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (c.id) c.id, m.id, g.full_name, m.occurred_at
		  FROM conversation_claim c
		  JOIN activity src ON src.id = c.source_activity_id
		  JOIN activity m ON m.kind = 'meeting' AND m.archived_at IS NULL AND m.id <> src.id
		   AND `+relstrength.MeetingNotCalledOffSQL("m")+` AND `+occurredday.FromSQL("m", "src.occurred_at")+` AND (%s)
		  JOIN activity_participant ap ON ap.activity_id = m.id AND ap.role = 'attendee'
		   AND ap.response = 'accepted' AND ap.contact_id IS NOT NULL AND `+auth.ParticipantStandsSQL("ap", "m")+`
		  JOIN contact g ON g.id = ap.contact_id AND g.archived_at IS NULL AND g.id <> c.contact_id AND (%s)
		 WHERE c.id = ANY($%d) AND c.body ~* '\yinvit'
		   AND c.body ~* ('\y' || regexp_replace(trim(g.full_name), '([^[:alnum:][:space:]])', '\\\1', 'g') || '\y')
		   AND NOT EXISTS (SELECT 1 FROM regexp_split_to_table(trim(g.full_name), '\s+') part WHERE length(part) < 3)
		   AND (EXISTS (SELECT 1 FROM activity_participant mine WHERE mine.activity_id = m.id AND mine.contact_id = c.contact_id)
		        OR EXISTS (SELECT 1 FROM activity_link mine WHERE mine.activity_id = m.id AND mine.contact_id = c.contact_id))
		   AND EXISTS (SELECT 1 FROM relationship theirs JOIN relationship ours
		                 ON ours.company_id = theirs.company_id AND ours.kind = 'employment'
		                AND `+employment.IsCurrentSQL("ours.ended_at")+` AND ours.archived_at IS NULL
		                WHERE theirs.contact_id = g.id AND theirs.kind = 'employment'
		                  AND `+employment.IsCurrentSQL("theirs.ended_at")+` AND theirs.archived_at IS NULL
		                  AND ours.contact_id = c.contact_id)
		 ORDER BY c.id, m.occurred_at, m.id`, meetingScope, guestScope, claimPos), args...)
	if err != nil {
		return fmt.Errorf("find the promises that look kept: %w", err)
	}
	type found struct {
		Claim, Meeting ids.UUID
		Guest          string
		At             time.Time
	}
	hits, err := pgx.CollectRows(rows, pgx.RowToStructByPos[found])
	if err != nil {
		return fmt.Errorf("find the promises that look kept: %w", err)
	}
	for _, hit := range hits {
		claims[at[hit.Claim]].LooksKept = &struct {
			GuestName string             `json:"guest_name"`
			MeetingAt time.Time          `json:"meeting_at"`
			MeetingId openapi_types.UUID `json:"meeting_id"`
		}{GuestName: hit.Guest, MeetingAt: hit.At, MeetingId: openapi_types.UUID(hit.Meeting)}
	}
	return nil
}
