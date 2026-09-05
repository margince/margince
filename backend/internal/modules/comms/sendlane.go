// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The shape both Worklist send-lanes are.
//
// A bounce lane and an undelivered lane ask the same question of the same
// table — which of MY sends went wrong, recently, and who is each one about —
// and differ in three words: which column records that it went wrong, which
// records when, and which sends count. Written twice they drifted in the ways
// that matter least and are hardest to see: one capping the subject line and
// the other not, one carrying the visibility clause on the person join and the
// other forgetting it.
//
// So the statement is written once with those three words named, and each lane
// supplies its own. Every fragment is still a literal — nothing here is built
// from a value that reached the process at runtime.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// alwaysVisible is what an UNBOUNDED reader's row scope reduces to. The
// visibility helper answers the empty string for a caller nothing narrows, and
// an empty string spliced into a WHERE is a syntax error — so a read
// substitutes a predicate that is simply true rather than reshaping the
// statement around the gap.
const alwaysVisible = "TRUE"

// subjectLineBound caps the send's subject on the way to the wire, as the
// sibling lanes cap their free text: the column is unbounded and eight
// multi-megabyte headlines would be a self-inflicted flood.
const subjectLineBound = 300

// sendLane names the three things one lane differs from its sibling by. Each
// field is a SQL fragment spelled as a constant at the lane that owns it.
type sendLane struct {
	// reasonColumn holds the words the card shows under the subject line.
	reasonColumn string
	// atColumn is the stamp the lane windows and orders on.
	atColumn string
	// only is what makes a row this lane's business rather than the other's.
	only string
	// recipientColumn names the address the card reports, or is empty where the
	// lane has none to name. A bounce report names ONE failed address and the
	// row records it; a park is about the send as a whole and has no such
	// column, so that lane says nothing about where it was aimed rather than
	// naming a recipient the failure was not about.
	recipientColumn string
}

// recipientExpr is the column the lane reads its address from, or a literal
// empty string where it has none. A literal rather than a NULL so the scan
// target stays a plain string on both lanes.
func (l sendLane) recipientExpr() string {
	if l.recipientColumn == "" {
		return "''"
	}
	return "COALESCE(o." + l.recipientColumn + ", '')"
}

// laneSend is one send on a lane: the five facts a card is drawn from.
type laneSend struct {
	ID       ids.UUID
	Subject  string
	Reason   string
	At       time.Time
	PersonID ids.UUID
	// Recipient is the address the lane's failure was about, or empty where the
	// lane names none. Never derived from the send's recipient list: a report
	// names ONE address, and a send carrying a CC would otherwise blame the
	// first name on it for a refusal that came from another.
	Recipient string
}

// statement joins each send to the person its activity is filed under.
// activity_link belongs to the activities module; this read joins it directly
// rather than through a port for the same reason consent's verdict read and
// deals' health read do — the link row is shared metadata every module's
// row-level reads resolve in their own statement. The join carries
// auth.LinkTargetVisibleClause, the clause the activities module's own link
// projections ask: owning the send says nothing about the visibility of the
// people its activity touches, and a person this caller may not read must not
// reach the wire even as a bare id. LATERAL with LIMIT 1 rather than a plain
// join: an activity filed under several people must not put the same send on
// the lane twice.
func (l sendLane) statement(ctx context.Context, userID ids.UUID, since time.Time, limit int, args *[]any) (string, error) {
	// Every placeholder is derived from the arg slice — the visibility clause
	// appends its own, and a hand-numbered $N beside a derived one drifts the
	// day the filter gains an argument.
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	visible, err := auth.LinkTargetVisibleClause(ctx, "al", arg)
	if err != nil {
		return "", err
	}
	if visible == "" {
		visible = alwaysVisible
	}
	return fmt.Sprintf(`
SELECT o.id, left(COALESCE(o.subject, ''), %d), COALESCE(o.%s, ''), o.%s, l.person_id,
       %s
  FROM comms_outbound o
  LEFT JOIN LATERAL (
    SELECT al.person_id FROM activity_link al
     WHERE al.activity_id = o.activity_id AND al.entity_type = 'person' AND `+visible+`
     ORDER BY al.person_id LIMIT 1
  ) l ON true
 WHERE o.user_id = $%d
   AND %s
   AND o.%s >= $%d
 ORDER BY o.%s DESC, o.id DESC
 LIMIT $%d`,
		subjectLineBound, l.reasonColumn, l.atColumn, l.recipientExpr(),
		arg(userID), l.only, l.atColumn, arg(since), l.atColumn, arg(limit)), nil
}

// read answers the calling person's own sends on this lane since `since`,
// newest first, bounded. The person comes from the bound principal and is not
// a parameter — another person's sends cannot be expressed — and a caller with
// no person behind it is refused with the permission sentinel, which the
// attention feed renders as a withheld lane.
//
// `what` names the lane in the two errors, so a refusal and a query fault say
// which read they came from rather than both saying "sends".
func (s *Store) readSendLane(ctx context.Context, lane sendLane, what string, since time.Time, limit int) ([]laneSend, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, fmt.Errorf("comms: reading your %s needs an authenticated person: %w", what, apperrors.ErrPermissionDenied)
	}
	// A send is an activity, and reading one back — subject line included —
	// carries the activity read grant like every other timeline read. After
	// the person check, so a caller with nobody behind it gets the sentinel
	// the lane withholds on rather than a bare unauthenticated error.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	statement, err := lane.statement(ctx, actor.UserID, since, limit, &args)
	if err != nil {
		return nil, err
	}
	var sends []laneSend
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, statement, args...)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		sends = []laneSend{}
		for rows.Next() {
			var send laneSend
			var person *ids.UUID
			if scanErr := rows.Scan(&send.ID, &send.Subject, &send.Reason, &send.At, &person, &send.Recipient); scanErr != nil {
				return scanErr
			}
			if person != nil {
				send.PersonID = *person
			}
			sends = append(sends, send)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("comms: listing %s: %w", what, err)
	}
	return sends, nil
}
