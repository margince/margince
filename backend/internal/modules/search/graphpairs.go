// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// Whether one contact has been reached by a current colleague, as this reader
// can prove it.
const (
	ReachReached    = "reached"
	ReachNotReached = "not_reached"
	ReachUnknown    = "unknown"
)

// PairDates is the newest readable exchange of one current colleague with one
// contact, by direction. A nil date means no readable event of that kind.
type PairDates struct {
	UserID, ContactID                        ids.UUID
	LastInboundAt, LastOutboundAt, LastMetAt *time.Time
	// How to read each date: "day" for an entry that names only its date.
	InboundPrecision, OutboundPrecision, MetPrecision string
}

// PairReading is what a reader may say about who talks to a contact set.
type PairReading struct {
	Pairs []PairDates
	// Reach has one entry per asked contact.
	Reach map[ids.UUID]string
}

// ReadablePairDates dates each current colleague's exchanges with the given
// contacts from the base activities, in one query. It keeps only activities
// the caller's content gate admits. The edge projection holds only
// workspace-audience mail and lags an audience change, so it cannot answer this.
//
// A contact is `not_reached` only when no linked activity exists. One the
// caller cannot read, or one with a colleague who has left, makes it `unknown`.
//
// The caller has passed the contact row scope for every id and holds the
// activity grant.
func ReadablePairDates(ctx context.Context, tx pgx.Tx, contacts []ids.UUID) (PairReading, error) {
	out := PairReading{Reach: make(map[ids.UUID]string, len(contacts))}
	for _, contact := range contacts {
		out.Reach[contact] = ReachNotReached
	}
	if len(contacts) == 0 {
		return out, nil
	}
	args := []any{contacts}
	arg := func(v any) int { args = append(args, v); return len(args) }
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH links AS (
		    SELECT up.user_id AS pair_user, pp.contact_id AS pair_contact, %[1]s,
		           %[2]s AS met, (%[3]s) AS live, (%[4]s) AS readable
		      FROM activity_participant up
		      JOIN activity_participant pp ON pp.activity_id = up.activity_id
		      JOIN activity a ON a.id = up.activity_id AND a.archived_at IS NULL
		       AND %[5]s AND %[6]s
		      JOIN app_user u ON u.id = up.user_id
		     WHERE up.user_id IS NOT NULL AND up.role IN %[7]s
		       AND pp.contact_id = ANY($1) AND pp.role IN %[7]s
		)
		SELECT c.pair_user, c.pair_contact,
		       max(c.occurred_at) FILTER (WHERE c.readable AND c.live AND %[8]s),
		       (array_agg(c.occurred_precision ORDER BY c.occurred_at DESC) FILTER (WHERE c.readable AND c.live AND %[8]s))[1],
		       max(c.occurred_at) FILTER (WHERE c.readable AND c.live AND %[9]s),
		       (array_agg(c.occurred_precision ORDER BY c.occurred_at DESC) FILTER (WHERE c.readable AND c.live AND %[9]s))[1],
		       max(c.occurred_at) FILTER (WHERE c.readable AND c.live AND c.met),
		       (array_agg(c.occurred_precision ORDER BY c.occurred_at DESC) FILTER (WHERE c.readable AND c.live AND c.met))[1],
		       bool_or(NOT c.readable OR NOT c.live)
		  FROM links c
		 GROUP BY c.pair_user, c.pair_contact
		 ORDER BY c.pair_contact, c.pair_user`,
		relstrength.FoldColumnsSQL("a"), graphMet, workforce.LiveMemberSQL("u"), readable,
		graphCountedActivity, auth.ParticipantStandsSQL("up", "a"), interactionRoles,
		relstrength.TheyWroteSQL("c.pair_contact", "c"), relstrength.WeWroteSQL("c")), args...)
	if err != nil {
		return out, fmt.Errorf("search: reading who talks to a contact set: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pair PairDates
		var withheld bool
		var inPrecision, outPrecision, metPrecision *string
		if err := rows.Scan(&pair.UserID, &pair.ContactID,
			&pair.LastInboundAt, &inPrecision, &pair.LastOutboundAt, &outPrecision,
			&pair.LastMetAt, &metPrecision, &withheld); err != nil {
			return out, fmt.Errorf("search: reading a pair of colleague and contact: %w", err)
		}
		pair.InboundPrecision, pair.OutboundPrecision, pair.MetPrecision =
			stated(inPrecision), stated(outPrecision), stated(metPrecision)
		if pair.LastInboundAt != nil || pair.LastOutboundAt != nil || pair.LastMetAt != nil {
			out.Pairs = append(out.Pairs, pair)
			out.Reach[pair.ContactID] = ReachReached
		} else if out.Reach[pair.ContactID] == ReachNotReached {
			out.Reach[pair.ContactID] = ReachUnknown
		}
	}
	return out, rows.Err()
}

// stated reads a nullable precision column; "" means no date of that kind.
func stated(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
