// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Who may CHANGE an activity, which is a different question from who may
// change a record.
//
// Its own file because an activity has no owner_id: every other write gate
// next door asks whose row this is, and none of those questions have an answer
// here. What an activity has instead is the set of records it is linked to,
// and authority over any one of them is authority over it — so the arms below
// are about authorship, assignment and the link walk rather than ownership.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EnsureActivityWritable is EnsureWritable for an activity, which has no
// owner_id of its own. The caller must READ it (the content gate — a limited
// conversation is nobody else's to edit), and their authority to CHANGE it is
// any of:
//
//   - they authored or captured it (captured_by names their user id);
//   - it is their task or their meeting (assignee_id / host_user_id);
//   - it is a link-less, workspace-shared note;
//   - at least one linked record is theirs to change — the same own/team
//     scope or `write` grant EnsureWritable takes on that record.
//
// Reads of customer identity are shared across the workspace, so the read
// gate alone would let every seat rewrite every colleague's correspondence;
// this is the arm that keeps activity writes team-shaped. An unbounded human
// edits every activity they can read, as they edit every record.
func EnsureActivityWritable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return EnsureActivityWritableIn(ctx, tx, id, true)
}

// EnsureActivityWritableIn is EnsureActivityWritable against a chosen row
// liveness. live=false serves a caller that already resolved the row past
// its own LiveOnly lock and confirmed it is held under a statutory
// retention obligation (activities.lockActivityForWrite).
//
// It skips the content-visible gate's LIVENESS half rather than passing live
// through to it: ActivityAvailableClause is `restricted_at IS NULL`
// UNCONDITIONALLY — by design, a restricted row reads as gone to everyone
// through that gate, live argument or not (ensureActivity's own doc). A
// caller reaching this function with live=false already proved the row
// exists by another means (the row lock, taken directly against the table),
// so re-asking the liveness half would only reproduce the same false 404
// this exists to remove. What it does NOT earn a skip from is the OTHER
// half ActivityContentClause folds in for every non-system caller —
// ActivityAudienceArm, the row's own participants/selected narrowing —
// which the ownership check below cannot stand in for: ownership answers
// "is this the caller's team's record", audience answers "did a human limit
// who reads this ONE message", and a caller who owns a record is not
// thereby a participant on every limited message under it. An unbounded
// human is bound by this too — ActivityContentClause's own doc says only
// the system principal reads the audience arm away, so Unbounded below must
// not become a bypass a held row's write-authority check does not have to
// answer for.
func EnsureActivityWritableIn(ctx context.Context, tx pgx.Tx, id ids.UUID, live bool) error {
	if live {
		if err := ensureActivity(ctx, tx, id, ActivityContentClause, true); err != nil {
			return err
		}
	} else {
		included, err := activityAudienceIncludes(ctx, tx, id)
		if err != nil {
			return err
		}
		if !included {
			return apperrors.ErrNotFound
		}
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if Unbounded(p) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, me, author := arg(id), arg(p.UserID), arg("%:"+p.UserID.String())

	var permitted bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%[1]d AND (
		   a.captured_by LIKE $%[3]d
		   OR a.assignee_id = $%[2]d
		   OR a.host_user_id = $%[2]d
		   OR NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id)
		   OR EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND %[4]s)))`,
		idPos, me, author, linkTargetWritable(p, "l", arg)), args...).Scan(&permitted); err != nil {
		return err
	}
	if !permitted {
		if !live {
			return apperrors.ErrNotFound
		}
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// activityAudienceIncludes probes ONLY ActivityAudienceArm — no liveness, no
// discoverability — for a caller in EnsureActivityWritableIn's live=false
// branch, whose row existence and archived state were already settled by
// its own lock. A row the caller cannot find at all answers false, not an
// error: the same not-found the audience arm itself would give inside the
// ordinary content-visible probe.
func activityAudienceIncludes(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	audience, err := ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	var included bool
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND (%s))`, idPos, audience),
		args...).Scan(&included)
	return included, err
}

// linkTargetWritable is linkTargetVisible's write twin: one arm per
// activity_link column, each asking whether the record it points at is the
// caller's to change.
func linkTargetWritable(p principal.Principal, alias string, arg func(any) int) string {
	arms := make([]string, 0, len(linkTargetTables))
	for _, t := range []struct{ column, table, probe string }{
		{contactIDColumn, tableContact, "wp"},
		{companyIDColumn, tableCompany, "wo"},
		{dealIDColumn, tableDeal, "wd"},
		{leadIDColumn, tableLead, "wl"},
		{projectIDColumn, tableProject, "wpr"},
	} {
		arms = append(arms, fmt.Sprintf(
			`(%[1]s.%[2]s IS NOT NULL AND EXISTS (SELECT 1 FROM %[3]s %[4]s WHERE %[4]s.id = %[1]s.%[2]s AND %[5]s))`,
			alias, t.column, t.table, t.probe, writeAuthorityPredicateAs(p, t.table, t.probe, arg)))
	}
	return "(" + strings.Join(arms, " OR ") + ")"
}
