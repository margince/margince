// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two reads every strength fold is made of. The recency read takes plain
// maxima over the whole history; only the window read counts, and it reads
// only the 90 days.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// strengthAdmittedSQL admits a fold row `a` at the instant now: an interaction
// that has happened by then, not archived, that every member may read.
func strengthAdmittedSQL(now string) string {
	return relstrength.InteractionCountsAtSQL("a", now) + " AND a.archived_at IS NULL" + auth.AudienceWorkspaceOnly("a")
}

// strengthRecencySQL is the history-wide aggregates for contact, in the order
// recencyDates scans them. They are the last interaction, the last engaged, and
// each direction's date with the precision of the activity it names.
func strengthRecencySQL(contact string) string {
	theyWrote, weWrote := relstrength.TheyWroteSQL(contact, "a"), relstrength.WeWroteSQL("a")
	return `max(a.occurred_at),
		       max(a.occurred_at) FILTER (WHERE ` + relstrength.ContactTookPartSQL(contact, "a") + `),
		       max(a.occurred_at) FILTER (WHERE ` + theyWrote + `),
		       ` + occurredday.LatestPrecisionSQL("a", theyWrote) + `,
		       max(a.occurred_at) FILTER (WHERE ` + weWrote + `),
		       ` + occurredday.LatestAnswerPrecisionSQL("a", weWrote)
}

// strengthWindowSQL is a lateral read of one contact's interactions inside the
// window, from the expression from to now. engaged is when the contact last
// took part, which the late counts start after. gate, when set, skips the read
// for a row whose dates show nothing in the window. The cited message is the
// newest inbound the window holds, so it agrees with the counts beside it.
func strengthWindowSQL(contact, engaged, from, now, gate, within string) string {
	if gate != "" {
		gate += " AND "
	}
	unit := `count(DISTINCT ` + strengthInteractionUnit + `)`
	theyWrote, weWrote := relstrength.TheyWroteSQL(contact, "a"), relstrength.WeWroteSQL("a")
	return `SELECT ` + unit + `,
		       ` + unit + ` FILTER (WHERE ` + theyWrote + `),
		       ` + unit + ` FILTER (WHERE ` + weWrote + `),
		       ` + unit + ` FILTER (WHERE ` + afterEngaged + `),
		       ` + unit + ` FILTER (WHERE ` + afterEngaged + ` AND ` + weWrote + `),
		       (array_agg(a.id ORDER BY a.occurred_at DESC, a.id DESC) FILTER (WHERE ` + theyWrote + `))[1]
		  FROM (SELECT ` + foldColumns + `, ` + engaged + ` AS engaged_at
		          FROM activity a
		          JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = ` + contact + `
		         WHERE ` + gate + strengthAdmittedSQL(now) + ` AND ` + occurredday.FromSQL("a", from) + within + `) a`
}

// windowCounts are the columns strengthWindowSQL returns, scanned in its order.
func (r *RelationshipStrength) windowCounts() []any {
	return []any{&r.InteractionCount90d, &r.Inbound90d, &r.Outbound90d, &r.lateCount90d, &r.lateOutbound90d, &r.LastInboundActivity}
}

// takeWindow copies the window counts of c onto r, leaving r's dates.
func (r *RelationshipStrength) takeWindow(c RelationshipStrength) {
	r.InteractionCount90d, r.Inbound90d, r.Outbound90d = c.InteractionCount90d, c.Inbound90d, c.Outbound90d
	r.lateCount90d, r.lateOutbound90d, r.LastInboundActivity = c.lateCount90d, c.lateOutbound90d, c.LastInboundActivity
}

// recencyDates are the columns strengthRecencySQL returns. The precisions
// arrive nullable and are kept as "" where their date is nil.
type recencyDates struct {
	inboundPrecision, outboundPrecision *string
}

func (d *recencyDates) targets(r *RelationshipStrength) []any {
	return []any{&r.LastInteraction, &r.LastEngaged, &r.LastInbound, &d.inboundPrecision, &r.LastOutbound, &d.outboundPrecision}
}

func (d *recencyDates) apply(r *RelationshipStrength) {
	r.LastInboundPrecision, r.LastOutboundPrecision = deref(d.inboundPrecision), deref(d.outboundPrecision)
}

// liveStrengths folds the contacts from their history at now: the recency read,
// then the window read for those whose last interaction falls in the window.
// A contact with nothing qualifying is absent.
func liveStrengths(
	ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, now time.Time, within *ids.ProjectID,
) (map[ids.ContactID]*RelationshipStrength, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos, nowPos := arg(contacts), arg(now)
	scoreWithin := ""
	if within != nil {
		scoreWithin = " AND " + activityWithinProject("a", arg(*within))
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT l.contact_id, `+strengthRecencySQL("l.contact_id")+`
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id
		 WHERE l.contact_id = ANY($%d) AND `+strengthAdmittedSQL(fmt.Sprintf("$%d::timestamptz", nowPos))+scoreWithin+`
		 GROUP BY l.contact_id`, contactsPos), args...)
	if err != nil {
		return nil, err
	}
	byContact := map[ids.ContactID]*RelationshipStrength{}
	windowStart := now.AddDate(0, 0, -relStrengthWindowDays)
	var warm []ids.ContactID
	var engaged []*time.Time
	for rows.Next() {
		var contactID ids.ContactID
		var dates recencyDates
		rs := &RelationshipStrength{}
		if err := rows.Scan(append([]any{&contactID}, dates.targets(rs)...)...); err != nil {
			rows.Close()
			return nil, err
		}
		dates.apply(rs)
		byContact[contactID] = rs
		if rs.LastInteraction != nil && !rs.LastInteraction.Before(windowStart) {
			warm, engaged = append(warm, contactID), append(engaged, rs.LastEngaged)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(warm) == 0 {
		return byContact, nil
	}
	return byContact, windowRead(ctx, tx, warm, engaged, windowStart, now, within, byContact)
}

// windowRead fills the window counts of each warm contact, paired with when
// it last took part.
func windowRead(
	ctx context.Context, tx pgx.Tx, warm []ids.ContactID, engaged []*time.Time,
	windowStart, now time.Time, within *ids.ProjectID, byContact map[ids.ContactID]*RelationshipStrength,
) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos, engagedPos := arg(warm), arg(engaged)
	from, at := fmt.Sprintf("$%d", arg(windowStart)), fmt.Sprintf("$%d::timestamptz", arg(now))
	scoreWithin := ""
	if within != nil {
		scoreWithin = " AND " + activityWithinProject("a", arg(*within))
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT w.contact_id, c.*
		  FROM unnest($%d::uuid[], $%d::timestamptz[]) AS w(contact_id, engaged_at)
		 CROSS JOIN LATERAL (`+strengthWindowSQL("w.contact_id", "w.engaged_at", from, at, "", scoreWithin)+`) c`,
		contactsPos, engagedPos), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.ContactID
		var counts RelationshipStrength
		if err := rows.Scan(append([]any{&contactID}, counts.windowCounts()...)...); err != nil {
			return err
		}
		if rs, ok := byContact[contactID]; ok {
			rs.takeWindow(counts)
		}
	}
	return rows.Err()
}
