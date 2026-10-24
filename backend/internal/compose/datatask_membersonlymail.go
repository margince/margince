// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// membersOnlyMailDataTask holds the stored mail between members that capture
// opened to the workspace before it placed the members_only hold. It asks the
// test capture asks (capture.MemberAddresses), against today's members.
func membersOnlyMailDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792838024_members_only_mail", Version: 1,
		Title: "Holding mail between colleagues", Requires: []string{"core/1788316744"},
		Steps: []datatask.Step{{
			Name: "hold_members_only_mail", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMembersOnlyCandidates, Batch: holdMembersOnlyMailBatch,
		}},
	}
}

// membersOnlyCandidate is one open captured mail and the two places its
// parties can be read from: the stored original, and the participant rows
// capture stamped from it.
type membersOnlyCandidate struct {
	ID           ids.UUID
	Payload      []byte
	Counterparty *string
	Stamped      []string
	StampedRows  int
}

func holdMembersOnlyMailBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, original.payload, a.counterparty_email,
		       ARRAY(SELECT lower(p.address) FROM activity_participant p
		              WHERE p.activity_id = a.id AND p.address IS NOT NULL),
		       (SELECT count(*)::int FROM activity_participant p WHERE p.activity_id = a.id)
		  FROM activity a
		  LEFT JOIN LATERAL (
		       SELECT rc.payload FROM raw_capture rc
		        WHERE rc.id = a.raw_capture_id
		           OR (a.raw_capture_id IS NULL AND rc.source_system = a.source_system AND rc.source_id = a.source_id)
		        LIMIT 1) original ON true
		 WHERE `+activities.MembersOnlyHoldable+` AND a.id > @after_activity
		 ORDER BY a.id LIMIT @page_size`,
		pgx.NamedArgs{"after_activity": after, "page_size": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[membersOnlyCandidate])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	members, err := capture.MemberAddressesTx(ctx, tx)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	var hold []ids.ActivityID
	for _, candidate := range candidates {
		scanned = append(scanned, candidate.ID)
		if parties, known := candidate.parties(); known && members.CoverEveryParty(parties) {
			hold = append(hold, ids.From[ids.ActivityKind](candidate.ID))
		}
	}
	held, err := activities.HoldMembersOnlyTx(ctx, tx, hold)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, held)
}

// parties is every address the mail named. The original's headers come
// first, Bcc included. Without a readable original the stamped rows stand in,
// but only below the participant cap: past it capture dropped outside names,
// and a list missing one would read as members-only. A row stamped with a seat
// and no address is a member, so it adds nothing to test.
func (c membersOnlyCandidate) parties() ([]string, bool) {
	if raw, err := capture.DecodeStoredOriginal(c.Payload); err == nil {
		if parsed, err := mailmap.Parse(raw, ""); err == nil && len(parsed.Addresses()) > 0 {
			return parsed.Addresses(), true
		}
	}
	if c.StampedRows >= connector.MaxParticipants {
		return nil, false
	}
	parties := c.Stamped
	if c.Counterparty != nil {
		parties = append(parties, *c.Counterparty)
	}
	return parties, true
}

func estimateMembersOnlyCandidates(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id FROM activity a WHERE "+activities.MembersOnlyHoldable+" LIMIT @estimate_cap) candidates",
		pgx.NamedArgs{"estimate_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
