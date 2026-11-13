// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One timeline entry per message across mailboxes.
//
// Every seat that captured a message keeps its own row. Capture files a seat
// that cannot prove the incumbent under connector.SeatScopedMailKey. A reader of
// several such rows would see one mail per mailbox. So a timeline groups them by
// the Message-ID behind their key and shows one.
//
// Only rows whose content this reader may open join a group. A withheld copy
// stays its own entry, so the count says nothing the reader could not read.
// The sender types the Message-ID, so copies must also agree on sender,
// subject, body, direction and second. A forgery differing in any stays apart.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SeatCopyReadable is the content arm a projection over SeatCopyFrom passes to
// ActivityProjection: the candidates already judged it once.
const SeatCopyReadable = "copies.copy_readable"

// SeatCopyColumns are the trailing columns ScanSeatCopyRow reads after the projection.
const SeatCopyColumns = ", copies.copy_count, copies.copy_ids"

// SeatCopyFrom renders a `FROM` clause that keeps one `activity a` row per
// message.
//
// candidates is the reader's `WHERE` over `activity a`, and contentArm its
// audience test there. The window runs over every candidate before the caller's
// `ORDER BY` and `LIMIT`, so a page never holds two copies. A keyset clause goes
// in the caller's outer `WHERE`, applied to the row shown.
func SeatCopyFrom(ctx context.Context, candidates, contentArm string, arg func(any) int) string {
	shared := connector.SharedMailKeySQL("a")
	reader := sprintf("$%d::text", arg(readerOrNobody(ctx).String()))
	return `(SELECT seat_copy.copy_of, seat_copy.copy_readable,
	  count(*) OVER copy_group AS copy_count,
	  array_agg(seat_copy.copy_of) OVER copy_group AS copy_ids,
	  row_number() OVER (copy_group ORDER BY seat_copy.own_copy DESC, seat_copy.created_at, seat_copy.copy_of) AS copy_rank
	FROM (SELECT a.id AS copy_of, a.created_at, (` + contentArm + `) AS copy_readable,
	    CASE WHEN a.kind = 'email' AND ` + shared + ` IS NOT NULL THEN jsonb_build_array(` + shared + `,
	      lower(a.counterparty_email), a.subject, md5(coalesce(a.body, '')), a.direction,
	      date_trunc('second', a.occurred_at))::text END AS shared_key,
	    coalesce(a.source_id = ` + shared + ` || '` + connector.MailSeatMarker + `' || ` + reader + `, false) AS own_copy
	  FROM activity a WHERE ` + candidates + `) seat_copy
	WINDOW copy_group AS (PARTITION BY CASE WHEN seat_copy.copy_readable AND seat_copy.shared_key IS NOT NULL
	  THEN 'message:' || seat_copy.shared_key ELSE 'activity:' || seat_copy.copy_of::text END)
	) copies JOIN activity a ON a.id = copies.copy_of AND copies.copy_rank = 1`
}

// SeatCopies is what ScanSeatCopyRow read for one shown row: how many readable
// rows it stands for, and their ids.
type SeatCopies struct {
	count int64
	ids   []ids.UUID
}

func (c *SeatCopies) dest() []any { return []any{&c.count, &c.ids} }

// stamp sets copies and the other copies' ids on the record only when it
// stands for more than itself, so every copy stays reachable.
func (c SeatCopies) stamp(a *crmcontracts.Activity) {
	if c.count <= 1 {
		return
	}
	n := int(c.count)
	a.Copies = &n
	others := make([]openapi_types.UUID, 0, len(c.ids)-1)
	for _, id := range c.ids {
		if id != ids.UUID(a.Id) {
			others = append(others, openapi_types.UUID(id))
		}
	}
	a.CopyIds = &others
}

// ScanSeatCopyRow reads one row of a projection over SeatCopyFrom.
func ScanSeatCopyRow(ctx context.Context, row pgx.Row) (crmcontracts.Activity, SeatCopies, error) {
	var copies SeatCopies
	a, err := scanActivity(ctx, storekit.TrailingColumns{Row: row, Dest: copies.dest()})
	if err != nil {
		return crmcontracts.Activity{}, SeatCopies{}, err
	}
	copies.stamp(&a)
	return a, copies, nil
}

// AttachCopyLinks adds to each shown row the links its other copies carry.
//
// Each colleague files their own copy, so the records it is about are the union
// over the copies. Every link keeps its own row-scope check, and a record two
// copies name shows once.
func AttachCopyLinks(ctx context.Context, tx pgx.Tx, activities []crmcontracts.Activity, copies []SeatCopies) error {
	owner := map[ids.UUID]int{}
	for i, group := range copies[:min(len(copies), len(activities))] {
		for _, id := range group.ids {
			if id != ids.UUID(activities[i].Id) {
				owner[id] = i
			}
		}
	}
	if len(owner) == 0 {
		return nil
	}
	return attachLinksFrom(ctx, tx, activities, owner, true)
}
