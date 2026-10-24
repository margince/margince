// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// membersOnlyMailDataTask holds the stored mail between members that capture
// opened to the workspace before it placed the members_only hold. It asks the
// test capture asks (capture.MemberAddresses), against today's members.
//
// Only the stored original names every party. The participant rows do not:
// capture records no sign that the cap dropped a crowded list, and mail
// stamped before Cc rows existed has none. So mail without a readable original
// is never held. The second step counts the mail with no original. A row whose
// original does not parse is logged by id and stays in the first step's
// unchanged count; only Go can tell that it does not parse.
func membersOnlyMailDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792838024_members_only_mail", Version: 1,
		Title: "Holding mail between colleagues", Requires: []string{"core/1788316744"},
		Steps: []datatask.Step{
			{
				Name: "hold_members_only_mail", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateMembersOnlyMail(true), Batch: holdMembersOnlyMailBatch,
			},
			{
				Name: "count_mail_with_unknown_parties", Limit: 500, MinLimit: 10,
				Audit:    datatask.AuditPolicy{Reason: "writes nothing: it counts the open mail no stored original names the parties of"},
				Estimate: estimateMembersOnlyMail(false), Batch: countUnknownPartiesBatch,
			},
		},
	}
}

// membersOnlyMailFrom is the open captured mail the hold may reach, joined to
// its stored original the way the recipient-restore task reads one.
const membersOnlyMailFrom = `FROM activity a
  LEFT JOIN LATERAL (
       SELECT rc.payload FROM raw_capture rc
        WHERE ` + storedMailOriginal + `
        LIMIT 1) original ON ` + storedMailProvider + `
 WHERE ` + activities.MembersOnlyHoldable

type membersOnlyCandidate struct {
	ID      ids.UUID
	Payload []byte
}

func holdMembersOnlyMailBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, original.payload `+membersOnlyMailFrom+`
		   AND original.payload IS NOT NULL AND a.id > @after_activity
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
		parties, readable := originalParties(candidate.Payload)
		if !readable {
			// Left open, like mail with no original: one corrupt row must not
			// stop the repair of every row behind it.
			slog.WarnContext(ctx, "members-only repair: stored original unreadable, row left open",
				"activity_id", candidate.ID)
			continue
		}
		if members.CoverEveryParty(parties) {
			hold = append(hold, ids.From[ids.ActivityKind](candidate.ID))
		}
	}
	held, err := activities.HoldMembersOnlyTx(ctx, tx, hold)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, held)
}

// originalParties is every address the original's headers name, Bcc
// included, and false when the original cannot be read.
func originalParties(payload []byte) ([]string, bool) {
	raw, err := capture.DecodeStoredOriginal(payload)
	if err != nil {
		return nil, false
	}
	parsed, err := mailmap.Parse(raw, "")
	if err != nil {
		return nil, false
	}
	return parsed.Addresses(), true
}

// countUnknownPartiesBatch pages the open mail without an original, so the
// step's scanned count is how many rows the hold could not judge.
func countUnknownPartiesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id `+membersOnlyMailFrom+`
		   AND original.payload IS NULL AND a.id > @after_activity
		 ORDER BY a.id LIMIT @page_size`,
		pgx.NamedArgs{"after_activity": after, "page_size": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	unknown, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, unknown, 0)
}

func estimateMembersOnlyMail(withOriginal bool) func(context.Context, datatask.Querier, int64) (int64, bool, error) {
	return func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
		if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
			return 0, false, err
		}
		var count int64
		err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+membersOnlyMailFrom+`
			   AND (original.payload IS NOT NULL) = @with_original LIMIT @estimate_cap) candidates`,
			pgx.NamedArgs{"with_original": withOriginal, "estimate_cap": maximum + 1}).Scan(&count)
		return min(count, maximum), count > maximum, err
	}
}
