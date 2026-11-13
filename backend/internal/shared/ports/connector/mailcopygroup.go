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
	shared := SharedMailKeySQL(alias)
	reader := fmt.Sprintf("$%d::text", arg(copyReader(ctx).String()))
	return `(SELECT seat_copy.copy_of, seat_copy.copy_readable,
	  count(*) OVER copy_group AS copy_count,
	  array_agg(seat_copy.copy_of) OVER copy_group AS copy_ids,
	  row_number() OVER (copy_group ORDER BY seat_copy.own_copy DESC, seat_copy.created_at, seat_copy.copy_of) AS copy_rank
	FROM (SELECT ` + alias + `.id AS copy_of, ` + alias + `.created_at, (` + contentArm + `) AS copy_readable,
	    CASE WHEN ` + alias + `.kind = 'email' AND ` + shared + ` IS NOT NULL THEN jsonb_build_array(` + shared + `,
	      lower(` + alias + `.counterparty_email), ` + alias + `.subject, md5(coalesce(` + alias + `.body, '')), ` + alias + `.direction,
	      date_trunc('second', ` + alias + `.occurred_at))::text END AS shared_key,
	    coalesce(` + alias + `.source_id = ` + shared + ` || '` + MailSeatMarker + `' || ` + reader + `, false) AS own_copy
	  FROM ` + source + ` ` + alias + ` WHERE ` + candidates + `) seat_copy
	WINDOW copy_group AS (PARTITION BY CASE WHEN seat_copy.copy_readable AND seat_copy.shared_key IS NOT NULL
	  THEN 'message:' || seat_copy.shared_key ELSE 'activity:' || seat_copy.copy_of::text END)
	) copies JOIN activity ` + alias + ` ON ` + alias + `.id = copies.copy_of AND copies.copy_rank = 1`
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
