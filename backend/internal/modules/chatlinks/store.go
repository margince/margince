// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The record kinds a channel can carry. Each is also the table it lives in.
const (
	RecordDeal    = "deal"
	RecordProject = "project"
)

// The verbosity levels, from loudest to quietest.
const (
	VerbosityEverything = "everything"
	VerbosityImportant  = "important"
	VerbosityMilestones = "milestones"
	VerbositySilent     = "silent"
)

// The link statuses: a gone channel pauses the link until it is replaced.
const (
	StatusActive      = "active"
	StatusChannelGone = "channel_gone"
)

// Link is one record's channel.
type Link struct {
	ID                ids.UUID
	ConnectionID      ids.UUID
	Provider          string
	ChannelSlug       string
	ChannelName       string
	DealID            *ids.UUID
	ProjectID         *ids.UUID
	Verbosity         string
	CardMessageID     string
	CardFingerprint   string
	HistoryCursor     *int64
	BackfillRequested bool
	Status            string
	Version           int64
	CreatedAt         time.Time
}

// FlushEnqueuer schedules the post that carries a link's pending items, in the
// transaction that recorded them. compose implements it over River, with the
// job unique per link so ten events make one post.
type FlushEnqueuer interface {
	EnqueueFlushTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) error
}

// Viewer is a live member, by the address the chat knows them by.
type Viewer struct {
	ID    ids.UUID
	Email string
}

// Viewers names, in tx, every live human member who may open the record under
// their own rights. compose implements it over the members' live grants and
// the row gate, which this package cannot reach.
type Viewers func(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) ([]Viewer, error)

// Store owns chat_link and chat_post_item.
type Store struct {
	db      *database.DB
	chats   Chats
	flush   FlushEnqueuer
	viewers Viewers
}

// NewStore wires the store over the chat it acts in, the flush scheduler, and
// who a new channel may be opened to.
func NewStore(db *database.DB, chats Chats, flush FlushEnqueuer, viewers Viewers) *Store {
	return &Store{db: db, chats: chats, flush: flush, viewers: viewers}
}

const linkColumns = `id, connection_id, provider, channel_slug, channel_name, deal_id, project_id, verbosity,
	coalesce(card_message_id, ''), coalesce(card_fingerprint, ''), history_cursor, backfill_requested, status, version, created_at`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.ConnectionID, &l.Provider, &l.ChannelSlug, &l.ChannelName, &l.DealID, &l.ProjectID,
		&l.Verbosity, &l.CardMessageID, &l.CardFingerprint, &l.HistoryCursor, &l.BackfillRequested,
		&l.Status, &l.Version, &l.CreatedAt)
	return l, err
}

// recordColumn is the chat_link column that names a record of kind.
func recordColumn(kind string) (string, error) {
	switch kind {
	case RecordDeal:
		return "deal_id", nil
	case RecordProject:
		return "project_id", nil
	}
	return "", fmt.Errorf("a channel is linked to a deal or a project: %w", apperrors.ErrInvalidArgument)
}

// liveLinkOf reads the record's live link, or ErrLinkNotFound.
func liveLinkOf(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (Link, error) {
	column, err := recordColumn(kind)
	if err != nil {
		return Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM chat_link
		WHERE `+column+` = $1 AND archived_at IS NULL`, id))
	if err == pgx.ErrNoRows {
		return Link{}, ErrLinkNotFound
	}
	return link, err
}

// auditImage is the audit trail's view of a link. The card and cursor stay
// out: they are sync state, not a decision anyone made.
func auditImage(l Link) map[string]any {
	return map[string]any{
		"provider":     l.Provider,
		"channel_slug": l.ChannelSlug,
		"channel_name": l.ChannelName,
		"deal_id":      l.DealID,
		"project_id":   l.ProjectID,
		"verbosity":    l.Verbosity,
		"status":       l.Status,
	}
}
