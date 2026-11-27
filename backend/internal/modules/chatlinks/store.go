// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The record kinds a channel can carry. Each is also the table it lives in.
const (
	RecordCompany = "company"
	RecordDeal    = "deal"
	RecordProject = "project"
)

// The verbosity levels, from loudest to quietest.
const (
	MessageDetailBrief  = "brief"
	MessageDetailFull   = "full"
	MessageDetailLegacy = "legacy"
	VerbosityEverything = "everything"
	VerbosityImportant  = "important"
	VerbosityMilestones = "milestones"
	VerbositySilent     = "silent"
)

// The link statuses: a gone channel pauses the link until it is replaced.
const (
	StatusActive              = "active"
	StatusChannelGone         = "channel_gone"
	StatusIdentityNeedsReview = "identity_needs_review"
)

// Link is one record's channel.
type Link struct {
	WorkspaceName       string
	CardPaused          bool
	CardGeneration      string
	NativeIdentity      bool
	ID                  ids.UUID
	ConnectionID        ids.UUID
	Provider            string
	ChannelSlug         string
	ChannelName         string
	RootID              string
	IncludeHistory      bool
	PostSupported       bool
	CompanyID           *ids.UUID
	DealID              *ids.UUID
	ProjectID           *ids.UUID
	Verbosity           string
	MessageDetail       string
	storedMessageDetail *string
	CardMessageID       string
	CardFingerprint     string
	HistoryCursor       *int64
	BackfillRequested   bool
	ReadDisabled        bool
	PostDisabled        bool
	ReadSince           *time.Time
	Status              string
	Version             int64
	CreatedAt           time.Time
}

// Scheduler queues a link's jobs in the transaction that asked for them.
// compose implements it over River. The flush posts a link's pending items and
// is unique per link, so ten events make one post. The pull reads a bot's
// channels and is unique per bot.
type Scheduler interface {
	EnqueueFlushTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) error
	EnqueuePullTx(ctx context.Context, tx pgx.Tx, connectionID ids.UUID) error
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
	historyProfiles HistoryProfileReader
	vault           func() keyvault.Vault
	memberClient    MemberClient
	memberRefresh   MemberRefresh
	memberAuthority MemberAuthority
	messengerRoster MessengerRoster
	db              *database.DB
	chats           Chats
	jobs            Scheduler
	viewers         Viewers
}

// NewStore wires the store over the chat it acts in, the job scheduler, and
// who a new channel may be opened to.
func NewStore(db *database.DB, chats Chats, jobs Scheduler, viewers Viewers) *Store {
	return &Store{db: db, chats: chats, jobs: jobs, viewers: viewers}
}

// WithVault keeps provider credentials behind the installation's key provider.
func (s *Store) WithVault(vault func() keyvault.Vault) *Store {
	s.vault = vault
	return s
}

const linkColumns = `id, connection_id, provider, channel_slug, channel_name, deal_id, project_id, verbosity,
	coalesce(card_message_id, ''), coalesce(card_fingerprint, ''), history_cursor, backfill_requested, status, version, created_at, company_id, read_disabled, post_disabled, read_since, message_detail,coalesce(root_id,''),include_history,post_supported,native_identity,card_paused,coalesce(card_generation::text,'')`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.ConnectionID, &l.Provider, &l.ChannelSlug, &l.ChannelName, &l.DealID, &l.ProjectID,
		&l.Verbosity, &l.CardMessageID, &l.CardFingerprint, &l.HistoryCursor, &l.BackfillRequested,
		&l.Status, &l.Version, &l.CreatedAt, &l.CompanyID, &l.ReadDisabled, &l.PostDisabled, &l.ReadSince, &l.storedMessageDetail, &l.RootID, &l.IncludeHistory, &l.PostSupported, &l.NativeIdentity, &l.CardPaused, &l.CardGeneration)
	switch {
	case l.storedMessageDetail != nil:
		l.MessageDetail = *l.storedMessageDetail
	case l.Verbosity == VerbosityEverything:
		l.MessageDetail = MessageDetailFull
	default:
		l.MessageDetail = MessageDetailBrief
	}
	return l, err
}

// recordColumn is the chat_link column that names a record of kind.
func recordColumn(kind string) (string, error) {
	switch kind {
	case RecordCompany:
		return companyIDKey, nil
	case RecordDeal:
		return "deal_id", nil
	case RecordProject:
		return "project_id", nil
	}
	return "", fmt.Errorf("a channel is linked to a company, deal or project: %w", apperrors.ErrInvalidArgument)
}

// liveLinkOf reads the record's live link, or ErrLinkNotFound.
func liveLinkOf(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (Link, error) {
	return readLiveLink(ctx, tx, kind, id, "")
}

// heldLiveLinkOf is liveLinkOf holding the row until tx ends, so the version
// it read is still the row's when the caller writes under it.
func heldLiveLinkOf(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (Link, error) {
	return readLiveLink(ctx, tx, kind, id, " FOR UPDATE")
}

func readLiveLink(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, lock string) (Link, error) {
	column, err := recordColumn(kind)
	if err != nil {
		return Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM chat_link
		WHERE `+column+` = $1 AND archived_at IS NULL`+lock, id))
	if err == pgx.ErrNoRows {
		return Link{}, ErrLinkNotFound
	}
	return link, err
}

// auditImage is the audit trail's view of a link. The card and cursor stay
// out: they are sync state, not a decision anyone made.
func auditImage(l Link) map[string]any {
	return map[string]any{
		providerKey: l.Provider, "card_paused": l.CardPaused, "card_generation": l.CardGeneration,
		rootIDKey: l.RootID, "native_identity": l.NativeIdentity, "include_history": l.IncludeHistory, "post_supported": l.PostSupported, "read_since": l.ReadSince,
		"channel_slug":   l.ChannelSlug,
		"channel_name":   l.ChannelName,
		"company_id":     l.CompanyID,
		"deal_id":        l.DealID,
		"project_id":     l.ProjectID,
		"verbosity":      l.Verbosity,
		"message_detail": l.storedMessageDetail,
		"status":         l.Status,
		"read_disabled":  l.ReadDisabled, "post_disabled": l.PostDisabled,
	}
}
