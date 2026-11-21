// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailCopyGroupFrom is the one SQL spelling of "which mailbox copies are one
// mail". It lives in this port because two modules list mail: activities
// (timelines, threads) and search (hits, the context graph). A reader that
// spelled its own key would show a mail twice, or join a forgery to it.
//
// It renders a `FROM` clause that keeps one row of source per message, joined
// back to `activity <alias>`. source is `activity` or a subquery over it with
// its columns; candidates is the reader's `WHERE` over alias; contentArm is its
// audience test there.
//
// Copies join only when the reader may open each and they agree on Message-ID,
// sender, subject, body, direction and second: the sender types the
// Message-ID. The copy shown is the reader's own, else the first filed.
func MailCopyGroupFrom(ctx context.Context, source, alias, candidates, contentArm string, arg func(any) int) string {
	return MailCopyGroupBoundedFrom(ctx, source, alias, candidates, contentArm, FoldBound{}, arg)
}

// FoldBound limits a fold to the newest candidates, for a reader that orders
// by `occurred_at DESC` and keeps one page. Copies of one mail agree on the
// second they happened, so a fold that leaves out the oldest second it read
// never splits a group.
type FoldBound struct {
	// Rows is how many of the newest candidates the fold reads. Zero folds
	// every candidate.
	Rows int
	// Ceiling is a SQL timestamptz: candidates must have happened before it.
	// A reader that continues from a cursor passes the second after the cursor's.
	Ceiling string
}

// CopyFoldFullColumn is the column of the fold that says it read every
// candidate. A reader that gets a short page from a bounded fold widens the
// bound only while this is false.
const CopyFoldFullColumn = "copy_fold_full"

// MailCopyGroupBoundedFrom is MailCopyGroupFrom over the candidates the bound
// keeps. Every group it returns is whole. Every group it leaves out happened
// before every group it returns, so the first rows of `ORDER BY occurred_at DESC`
// are the unbounded fold's first rows.
func MailCopyGroupBoundedFrom(ctx context.Context, source, alias, candidates, contentArm string, bound FoldBound, arg func(any) int) string {
	from, full := source+" "+alias+" WHERE "+candidates, "TRUE"
	if bound.Rows > 0 {
		if bound.Ceiling != "" {
			candidates = fmt.Sprintf("(%s) AND %s.occurred_at < %s -- occurred_at raw: a keyset bound on stored values\n", candidates, alias, bound.Ceiling)
		}
		// The oldest second read may hold copies the limit cut off, so only
		// the seconds above it are whole. An exhausted read has no such second.
		from = fmt.Sprintf(`(SELECT read.id, read.copy_full FROM (SELECT newest.id, count(*) OVER () < %[1]d AS copy_full,
	      date_trunc('second', newest.occurred_at) > min(date_trunc('second', newest.occurred_at)) OVER () AS copy_whole -- occurred_at raw: seconds among the rows read
	    FROM (SELECT %[2]s.id, %[2]s.occurred_at FROM %[3]s %[2]s WHERE %[4]s
	      ORDER BY %[2]s.occurred_at DESC, %[2]s.id DESC LIMIT %[1]d) newest) read
	    WHERE read.copy_whole OR read.copy_full) bounded JOIN activity %[2]s ON %[2]s.id = bounded.id WHERE TRUE`,
			bound.Rows, alias, source, candidates)
		full = "bounded.copy_full"
	}
	shared := SharedMailKeySQL(alias)
	reader := fmt.Sprintf("$%d::text", arg(copyReader(ctx).String()))
	// A rank filter can make Postgres expect one group and rescan activity for each row.
	// DISTINCT ON gives the join a group estimate instead.
	return `(SELECT DISTINCT ON (grouped.copy_key) grouped.copy_of, grouped.copy_readable, grouped.` + CopyFoldFullColumn + `,
	  count(*) OVER copy_group AS copy_count,
	  array_agg(grouped.copy_of) OVER copy_group AS copy_ids
	FROM (SELECT seat_copy.copy_of, seat_copy.created_at, seat_copy.copy_readable, seat_copy.own_copy, seat_copy.` + CopyFoldFullColumn + `,
	  CASE WHEN seat_copy.copy_readable AND seat_copy.shared_key IS NOT NULL
	  THEN 'message:' || seat_copy.shared_key ELSE 'activity:' || seat_copy.copy_of::text END AS copy_key
	  FROM (SELECT ` + alias + `.id AS copy_of, ` + alias + `.created_at, (` + contentArm + `) AS copy_readable,
	    CASE WHEN ` + alias + `.kind = 'email' AND ` + shared + ` IS NOT NULL THEN jsonb_build_array(` + shared + `,
	      lower(` + alias + `.counterparty_email), ` + alias + `.subject, md5(coalesce(` + alias + `.body, '')), ` + alias + `.direction,
	      date_trunc('second', ` + alias + `.occurred_at))::text END AS shared_key,
	    coalesce(` + alias + `.source_id = ` + shared + ` || '` + MailSeatMarker + `' || ` + reader + `, false) AS own_copy, ` + full + ` AS ` + CopyFoldFullColumn + `
	  FROM ` + from + `) seat_copy
	) grouped
	WINDOW copy_group AS (PARTITION BY grouped.copy_key)
	ORDER BY grouped.copy_key, grouped.own_copy DESC, grouped.created_at, grouped.copy_of
	) copies JOIN activity ` + alias + ` ON ` + alias + `.id = copies.copy_of`
}

// copyReader is the seat whose own copy a reader is shown: the user, or the
// user an agent acts for. The nil UUID owns no copy, so a system read shows the
// first filed.
func copyReader(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return ids.UUID{}
	}
	return actor.UserID
}
