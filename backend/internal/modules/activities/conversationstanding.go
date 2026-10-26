// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Anyone who may read a deal's thread may answer it. The composer still tells a
// rep who was never on the conversation, and who owns its deal.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ConversationStanding is the caller's place in the conversation a message
// belongs to. Like MailboxesFor, it labels the thread and grants nothing.
type ConversationStanding struct {
	// OnConversation is true when the caller is on the thread's From, To or
	// Cc. It is also true on the team of a readable deal the message is filed under.
	OnConversation bool
	// DealOwner owns the readable deal the message is filed under; nil when
	// there is none, or it has no owner.
	DealOwner *ids.UUID
}

// ConversationStandingFor reads only thread messages the caller may read, so a
// hidden message that names them says nothing.
func (s *Store) ConversationStandingFor(ctx context.Context, id ids.ActivityID) (ConversationStanding, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ConversationStanding{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ConversationStanding{}, errors.New("activities: no principal bound for the conversation standing")
	}
	var out ConversationStanding
	// One snapshot, so the content gate and the reads it guards see one state.
	err := s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		activity, err := readActivityContent(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		// A call, meeting, note or task is written about, not replied into: it has no thread to join.
		if slices.Contains(writtenAboutKinds, activity.Kind) {
			out = ConversationStanding{OnConversation: true}
			return nil
		}
		onThread, err := s.onThread(ctx, tx, id, actor.UserID)
		if err != nil {
			return err
		}
		deal, err := filedDealStanding(ctx, tx, id, actor.UserID)
		if err != nil {
			return err
		}
		out = ConversationStanding{OnConversation: onThread || deal.onTeam, DealOwner: deal.owner}
		return nil
	})
	return out, err
}

// onThread asks whether viewer's seat or one of their own addresses is a
// From, To or Cc of a readable message of the anchor's correspondence.
func (s *Store) onThread(ctx context.Context, tx pgx.Tx, anchor ids.ActivityID, viewer ids.UUID) (bool, error) {
	domain, err := s.correspondenceDomainTx(ctx, tx, anchor.UUID)
	if err != nil {
		return false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	identities, err := s.mailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return false, err
	}
	// The seat arm asks whether the participant row stands. Standing on a
	// thread decides what the caller may read next.
	//
	// A guessed mailbox owner would give that to a seat the message never
	// reached. The address arm needs no such test, because a guessed row
	// carries no address.
	me := fmt.Sprintf("$%d::uuid", arg(viewer))
	q := fmt.Sprintf(`
		SELECT EXISTS (
		  SELECT 1 FROM activity anchor
		    JOIN activity a ON a.id = anchor.id OR %s
		    JOIN activity_participant ap ON ap.activity_id = a.id AND ap.role IN ('from', 'to', 'cc')
		   WHERE anchor.id = $%d AND a.archived_at IS NULL AND %s
		     AND ((ap.user_id = %s AND %s) OR lower(ap.address) = ANY(%s)))`,
		SameCorrespondenceSQL("a", "anchor", fmt.Sprintf("$%d::text", arg(domain))),
		arg(anchor), content, me, auth.ParticipantStandsSQL("ap", "a"),
		mailSeatAddressesSQL(identities, me))
	var on bool
	if err := tx.QueryRow(ctx, q, args...).Scan(&on); err != nil {
		return false, fmt.Errorf("activities: reading whether the caller is on the thread: %w", err)
	}
	return on, nil
}

type dealStanding struct {
	onTeam bool
	owner  *ids.UUID
}

// filedDealStanding reads the deals the anchor is filed under that the caller
// may read. An open deal leads, then the latest filing: the business the
// conversation is about now.
func filedDealStanding(ctx context.Context, tx pgx.Tx, anchor ids.ActivityID, viewer ids.UUID) (dealStanding, error) {
	if !auth.ReadGranted(ctx, "deal") {
		return dealStanding{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	q := fmt.Sprintf(`
		SELECT d.owner_id, %s
		  FROM activity_link l
		  JOIN deal d ON d.id = l.deal_id AND d.archived_at IS NULL
		 WHERE l.activity_id = $%d AND l.entity_type = 'deal'`,
		storekit.DealTeamSQL("d", fmt.Sprintf("$%d::uuid", arg(viewer))), arg(anchor))
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return dealStanding{}, err
	}
	if scope != "" {
		q += ` AND (` + scope + `)`
	}
	q += ` ORDER BY d.status = 'open' DESC, l.created_at DESC, l.id DESC`
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return dealStanding{}, fmt.Errorf("activities: reading the deals a message is filed under: %w", err)
	}
	defer rows.Close()
	var out dealStanding
	for first := true; rows.Next(); first = false {
		var owner *ids.UUID
		var onTeam bool
		if err := rows.Scan(&owner, &onTeam); err != nil {
			return dealStanding{}, err
		}
		if first {
			out.owner = owner
		}
		out.onTeam = out.onTeam || onTeam
	}
	return out, rows.Err()
}
