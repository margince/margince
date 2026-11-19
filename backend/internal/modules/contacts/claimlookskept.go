// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A promise to invite someone looks kept when that someone accepted a later
// meeting. The finder here writes nothing. A claim with a responsible seat is
// closed from it by promisekept.go; any other claim only carries the hint.

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

// KeptEvidence is the accepted meeting that shows one promise kept.
type KeptEvidence struct {
	Claim, Meeting ids.UUID
	Guest          string
	At             time.Time
	Precision      crmcontracts.OccurredPrecision
}

func (e KeptEvidence) contract() *crmcontracts.ConversationClaimLooksKept {
	return &crmcontracts.ConversationClaimLooksKept{
		GuestName: e.Guest, MeetingAt: e.At, MeetingId: openapi_types.UUID(e.Meeting),
		MeetingPrecision: e.Precision,
	}
}

// withLooksKept sets `looks_kept` on the open promises of ours that a meeting
// this reader may open shows kept. The claims were read under the reader's
// scope, so their sources are readable. The meeting and guest use the same scope.
func withLooksKept(ctx context.Context, tx pgx.Tx, claims []crmcontracts.ConversationClaim) error {
	var open []ids.UUID
	at := map[ids.UUID]int{}
	for i, claim := range claims {
		if claim.Kind == crmcontracts.ConversationClaimKindCommitmentOurs && claim.Status == crmcontracts.ConversationClaimStatusOpen {
			open = append(open, ids.UUID(claim.Id))
			at[ids.UUID(claim.Id)] = i
		}
	}
	hits, err := FindKeptEvidence(ctx, tx, open)
	if err != nil {
		return err
	}
	for id, hit := range hits {
		claims[at[id]].LooksKept = hit.contract()
	}
	return nil
}

// FindKeptEvidence is the one matcher for "this promise to invite someone is
// kept". It answers, per claim, the earliest meeting that the reader in ctx may
// open, whatever the claim's status.
//
// The guest matches by full name on word boundaries, each part at least three
// letters, and works at the claim contact's company. The claim's own contact
// is on the meeting. The meeting starts after the promise, the guest accepted,
// and it is neither archived nor called off.
func FindKeptEvidence(ctx context.Context, tx pgx.Tx, open []ids.UUID) (map[ids.UUID]KeptEvidence, error) {
	if len(open) == 0 || !auth.ReadGranted(ctx, "activity") || !auth.ReadGranted(ctx, "contact") || !auth.ReadGranted(ctx, "relationship") {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimPos := arg(open)
	meetingScope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return nil, err
	}
	if meetingScope == "" {
		meetingScope = sqlAlwaysVisible
	}
	guestScope, err := auth.ScopeClauseFor(ctx, "contact", "g", arg)
	if err != nil {
		return nil, err
	}
	if guestScope == "" {
		guestScope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (c.id) c.id, m.id, g.full_name, m.occurred_at, m.occurred_precision
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
		return nil, fmt.Errorf("find the promises that look kept: %w", err)
	}
	hits, err := pgx.CollectRows(rows, pgx.RowToStructByPos[KeptEvidence])
	if err != nil {
		return nil, fmt.Errorf("find the promises that look kept: %w", err)
	}
	out := make(map[ids.UUID]KeptEvidence, len(hits))
	for _, hit := range hits {
		out[hit.Claim] = hit
	}
	return out, nil
}
