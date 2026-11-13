// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The tiers an event can post at. A link's verbosity admits a tier set.
const (
	TierMilestone  = "milestone"
	TierImportant  = "important"
	TierEverything = "everything"
)

// admits reports whether a link at verbosity posts an event of tier.
func admits(verbosity, tier string) bool {
	switch verbosity {
	case VerbosityEverything:
		return true
	case VerbosityImportant:
		return tier == TierMilestone || tier == TierImportant
	case VerbosityMilestones:
		return tier == TierMilestone
	}
	return false
}

// Event is one domain event bound for the channels of the records it touches.
// Detail holds only facts the template needs (a stage name, a changed field),
// never activity or chat text.
type Event struct {
	EventID    ids.UUID
	EventType  string
	EntityType string
	EntityID   ids.UUID
	ActivityID *ids.UUID
	Tier       string
	Detail     map[string]any
	Deals      []ids.UUID
	Projects   []ids.UUID
}

// Item is one pending or posted event of a link.
type Item struct {
	ID         ids.UUID
	EventType  string
	EntityType string
	EntityID   ids.UUID
	ActivityID *ids.UUID
	Detail     map[string]any
	CreatedAt  time.Time
}

// RecordEvent adds the event to every active link of the records it touches,
// and schedules a flush for each of them. (link_id, event_id) is the effect
// key, so a replayed event adds nothing. A link whose verbosity leaves the
// event out still gets a flush, because the card may show what changed.
func (s *Store) RecordEvent(ctx context.Context, ev Event) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	detail, err := json.Marshal(ev.Detail)
	if err != nil {
		return fmt.Errorf("chatlinks: encoding the detail of %s: %w", ev.EventType, err)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		links, err := linksTouching(ctx, tx, ev.Deals, ev.Projects)
		if err != nil {
			return err
		}
		for _, link := range links {
			if admits(link.Verbosity, ev.Tier) {
				if _, err := tx.Exec(ctx, `INSERT INTO chat_post_item
					(link_id, event_id, event_type, entity_type, entity_id, activity_id, tier, detail)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					ON CONFLICT DO NOTHING`,
					link.ID, ev.EventID, ev.EventType, ev.EntityType, ev.EntityID, ev.ActivityID, ev.Tier, detail); err != nil {
					return err
				}
			}
			if err := s.flush.EnqueueFlushTx(ctx, tx, link.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// linksTouching reads the active links of any of the records.
func linksTouching(ctx context.Context, tx pgx.Tx, deals, projects []ids.UUID) ([]Link, error) {
	if len(deals) == 0 && len(projects) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT `+linkColumns+` FROM chat_link
		WHERE archived_at IS NULL AND status = $3
		  AND (deal_id = ANY($1) OR project_id = ANY($2))`, deals, projects, StatusActive)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
}

// Pending is what one flush of a link has to do.
type Pending struct {
	Link Link
	// Items are the events no post has carried yet, oldest first.
	Items []Item
	// Rewrites are posted messages that lost an item to a narrowed audience,
	// each with the items it still carries.
	Rewrites []Rewrite
}

// Rewrite is a posted message to render again without its withdrawn items.
type Rewrite struct {
	MessageID string
	Items     []Item
}

// flushBatchLimit caps one post, so a burst becomes several readable posts.
const flushBatchLimit = 30

// PendingFor reads the link and what its next flush must post or rewrite. An
// archived link answers ErrLinkNotFound, so a flush queued before an unlink
// does nothing.
func (s *Store) PendingFor(ctx context.Context, linkID ids.UUID) (Pending, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return Pending{}, err
	}
	var out Pending
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		link, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM chat_link
			WHERE id = $1 AND archived_at IS NULL`, linkID))
		if err == pgx.ErrNoRows {
			return ErrLinkNotFound
		}
		if err != nil {
			return err
		}
		out.Link = link
		if out.Items, err = queryItems(ctx, tx, `link_id = $1 AND posted_at IS NULL AND withdrawn_at IS NULL
			ORDER BY created_at, id LIMIT `+fmt.Sprint(flushBatchLimit), linkID); err != nil {
			return err
		}
		out.Rewrites, err = rewritesOf(ctx, tx, linkID)
		return err
	})
	return out, err
}

func rewritesOf(ctx context.Context, tx pgx.Tx, linkID ids.UUID) ([]Rewrite, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT message_id FROM chat_post_item
		WHERE link_id = $1 AND withdrawn_at IS NOT NULL AND message_id IS NOT NULL`, linkID)
	if err != nil {
		return nil, err
	}
	messages, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make([]Rewrite, 0, len(messages))
	for _, message := range messages {
		items, err := queryItems(ctx, tx, `link_id = $1 AND message_id = $2 AND withdrawn_at IS NULL ORDER BY created_at, id`, linkID, message)
		if err != nil {
			return nil, err
		}
		out = append(out, Rewrite{MessageID: message, Items: items})
	}
	return out, nil
}

func queryItems(ctx context.Context, tx pgx.Tx, where string, args ...any) ([]Item, error) {
	rows, err := tx.Query(ctx, `SELECT id, event_type, entity_type, entity_id, activity_id, detail, created_at
		FROM chat_post_item WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		err := row.Scan(&it.ID, &it.EventType, &it.EntityType, &it.EntityID, &it.ActivityID, &it.Detail, &it.CreatedAt)
		return it, err
	})
}

// MarkPosted stamps the items one post carried with the post and the chat's
// message id. Withdrawn items are rewritten out of their post first, so only
// items still pending are stamped.
func (s *Store) MarkPosted(ctx context.Context, linkID, postID ids.UUID, messageID string, items []ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chat_post_item
			SET post_id = $2, message_id = NULLIF($3, ''), posted_at = now(), last_error = NULL
			WHERE link_id = $1 AND id = ANY($4) AND posted_at IS NULL`, linkID, postID, messageID, items)
		return err
	})
}

// MarkRewritten clears the message of the withdrawn items once their post was
// rewritten without them, so the rewrite is not done twice.
func (s *Store) MarkRewritten(ctx context.Context, linkID ids.UUID, messageID string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chat_post_item SET message_id = NULL
			WHERE link_id = $1 AND message_id = $2 AND withdrawn_at IS NOT NULL`, linkID, messageID)
		return err
	})
}

// RecordFailure counts a failed post on its items, keeping the error class
// only, never the chat's answer.
func (s *Store) RecordFailure(ctx context.Context, linkID ids.UUID, items []ids.UUID, reason string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chat_post_item SET attempts = attempts + 1, last_error = left($3, 500)
			WHERE link_id = $1 AND id = ANY($2)`, linkID, items, reason)
		return err
	})
}

// Withdraw takes an activity out of every channel it reached, because its
// audience narrowed. Its pending items never post; its posted items mark their
// post for a rewrite, and a flush is scheduled for each link it touched.
func (s *Store) Withdraw(ctx context.Context, activityID ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE chat_post_item SET withdrawn_at = now()
			WHERE activity_id = $1 AND withdrawn_at IS NULL RETURNING link_id`, activityID)
		if err != nil {
			return err
		}
		links, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, link := range links {
			if err := s.flush.EnqueueFlushTx(ctx, tx, link); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetCard records the card message and what it showed, so the next flush
// edits the card only when a shown fact changed.
func (s *Store) SetCard(ctx context.Context, linkID ids.UUID, messageID, fingerprint string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chat_link SET card_message_id = $2, card_fingerprint = $3
			WHERE id = $1 AND archived_at IS NULL`, linkID, messageID, fingerprint)
		return err
	})
}

// MarkChannelGone pauses a link whose channel the chat no longer has.
func (s *Store) MarkChannelGone(ctx context.Context, linkID ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM chat_link
			WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, linkID))
		if err == pgx.ErrNoRows || (err == nil && before.Status == StatusChannelGone) {
			return nil
		}
		if err != nil {
			return err
		}
		after, err := scanLink(tx.QueryRow(ctx, `UPDATE chat_link SET status = $2
			WHERE id = $1 RETURNING `+linkColumns, linkID, StatusChannelGone))
		if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", chatLinkEntity, linkID, auditImage(before), auditImage(after))
		return err
	})
}

// HasPending reports whether a link has items no post carried yet, so a flush
// that ran while events arrived knows to run once more.
func (s *Store) HasPending(ctx context.Context, linkID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var pending bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chat_post_item
			WHERE link_id = $1 AND posted_at IS NULL AND withdrawn_at IS NULL)`, linkID).Scan(&pending)
	})
	return pending, err
}
