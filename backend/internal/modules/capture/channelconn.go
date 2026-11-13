// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The workspace-level channel connection (telegram-oa design v2 §4): one bot
// binding per row, connected by an admin on behalf of the whole workspace
// rather than by one human over their own mailbox (which is what
// capture_connection models, and why channel_connection is a separate table —
// 0151_channel_connection.up.sql carries that reasoning).
//
// Connect's ORDERING is the load-bearing part of this file, and it is spelled
// out at Connect itself. Ingress PULLS (compose/telegrampoll.go), so there is no
// registration to make and no provider call after the insert: connect either
// succeeds as `connected` or writes nothing.
//
// The write is AUDIT-ONLY, the same ratified posture auditLifecycle documents
// for capture_connection (EVT-NOEVT-3): the closed event catalog
// (shared/kernel/events) defines no verb for a channel connection, so there is
// no event half to emit. Adding one is a spec change, not a local decision.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// channelConnectionObject is the RBAC object gating the channel-connection
// surface (identity/internal/policy coreObjects). Connecting a bot is
// destructive workspace-wide config — every seat's inbound Telegram traffic
// arrives through it — so create/update/delete are admin/ops-only while every
// role may read the status.
const channelConnectionObject = "channel_connection"

// ProviderTelegram is the provider whose check this module carries itself;
// compose adds the others through WithBotCheck.
const ProviderTelegram = telegram.ProviderName

// The channel_connection lifecycle states. There is deliberately no
// half-connected state: a pull ingress makes no provider call after the insert,
// so no row can be written that is not already live.
const (
	// channelStatusConnected: the binding is live and the dispatcher polls it.
	channelStatusConnected = "connected"
	// channelStatusDisconnected: the operator withdrew the binding. Captured
	// activities remain — disconnecting is not erasing.
	channelStatusDisconnected = "disconnected"
	// channelStatusError: the poll cannot proceed for a reason no retry repairs
	// — another consumer holds this bot's updates. The due-scan stops selecting
	// the row, so this is what actually parks it.
	channelStatusError = "error"
	// channelStatusReauthRequired: Telegram refused the sealed token. The admin
	// re-pastes it (ReplaceToken); nothing else can recover from here.
	channelStatusReauthRequired = "reauth_required"
)

// ChannelPollStopped names the two statuses a failing poll parks a connection
// under, so the poller in the composition layer states them by name rather than
// re-spelling the column's vocabulary.
const (
	ChannelPollStoppedByRivalConsumer = channelStatusError
	ChannelPollStoppedByBadToken      = channelStatusReauthRequired
)

// channelConnectionColumns is the read shape, spelled once so every scan
// agrees with every select.
const channelConnectionColumns = `id, provider, channel_id, channel_label, address, status, version, created_at, updated_at`

// ChannelConnection is one channel binding as read back. No vault ref rides this
// shape: the bot token lives sealed in the vault, addressed by a ref no caller
// here has a legitimate reason to hold — List, Get and Connect's own response all
// leave it out, and the poller reads it through its own narrow shape
// (ChannelPollTarget).
type ChannelConnection struct {
	ID        ids.UUID
	Provider  string
	ChannelID string
	// ChannelLabel is the bot's @username — display only. A bot's username is
	// mutable and re-assignable, so it identifies nothing; ChannelID (the
	// bot's global numeric id) is the key.
	ChannelLabel string
	// Address is the web address of a self-hosted provider's install, and
	// empty for a provider with one fixed API.
	Address   string
	Status    string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConnectRequest is Connect's input: which provider, and the BotFather token to
// seal. Everything else about the connection — the bot id, its label, the
// connecting human — is server-derived, so a caller cannot claim a bot identity
// it does not hold the token for.
type ConnectRequest struct {
	Provider string
	BotToken string
	Address  string
}

// ErrChannelWorkspaceBotAlreadyBound reports that this installation already
// holds a live bot, and so the remedy is to disconnect that binding.
//
// Only one live bot is permitted, because every outbound reply resolves the
// installation's bot and the send path refuses to guess between two — so a
// second binding would not add a channel, it would take away the ability to
// reply on either.
var ErrChannelWorkspaceBotAlreadyBound = errors.New("capture: this installation already has a live channel connection")

// ErrChannelWiringIncomplete reports that this deployment composed no credential
// custodian, so a bot token can neither be sealed nor destroyed. It is a
// DEPLOYMENT FACT an operator can act on, not an internal fault, so it refuses by
// name (503) instead of the opaque 500 an untyped error would become — which
// sends whoever is looking at the screen to a server log they may not have.
var ErrChannelWiringIncomplete = errors.New("capture: no credential custodian is composed for the channel surface")

// ChannelStore owns channel_connection and its write shape. vault is the
// custodian of the sealed bot token; api is the Telegram boundary.
//
// Connecting needs no public address of our own: a poll dials OUT, so this store
// has nothing to tell the provider about where we are.
//
// api is REQUIRED — every composition of this store supplies one, and a role
// that serves no channel surface composes no store at all rather than a
// client-less one. vault may be absent on a deployment that configured none, and
// every entry point that needs it refuses by name rather than proceeding
// half-wired.
type ChannelStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db    *database.DB
	vault keyvault.Vault
	bots  map[string]ChannelBotCheck
	log   *slog.Logger
}

// NewChannelStore wires the channel-connection store. vault may be nil when the
// deployment configured none — the mutating paths then refuse with a named,
// actionable error instead of writing a connection whose token nothing could
// unseal.
func NewChannelStore(db *database.DB, vault keyvault.Vault, api telegram.API, log *slog.Logger) *ChannelStore {
	if log == nil {
		log = slog.Default()
	}
	bots := map[string]ChannelBotCheck{ProviderTelegram: telegramBotCheck{api: api}}
	return &ChannelStore{db: db, vault: vault, bots: bots, log: log}
}

// withVault returns a copy of the store carrying the credential custodian. The
// composition root learns the vault from a later option than the one that built
// the store, and a copy keeps that late binding from mutating an instance a
// concurrent request could already be reading through.
func (s *ChannelStore) withVault(vault keyvault.Vault) *ChannelStore {
	next := *s
	next.vault = vault
	return &next
}

// Connect binds one bot to this workspace, and the order matters. The
// provider confirms the token before it is sealed, and Telegram's check also
// clears its webhook. So a refusal leaves no secret that nothing names.
// The row is inserted `connected` with its audit row last; no provider call
// follows, so there is no half-connected state.
func (s *ChannelStore) Connect(ctx context.Context, req ConnectRequest) (ChannelConnection, error) {
	if err := auth.Require(ctx, channelConnectionObject, principal.ActionCreate); err != nil {
		return ChannelConnection{}, err
	}
	// Human-only: the token grants read of every message the bot receives, so
	// an agent must not be able to bind one on its own initiative.
	if err := auth.RequireHuman(ctx); err != nil {
		return ChannelConnection{}, err
	}
	check, err := s.requireConnectWiring(req.Provider)
	if err != nil {
		return ChannelConnection{}, err
	}
	address, err := connectAddress(check, req.Address)
	if err != nil {
		return ChannelConnection{}, err
	}
	if err := check.ValidateToken(req.BotToken); err != nil {
		return ChannelConnection{}, err
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ChannelConnection{}, errors.New("capture: channel connect called outside a workspace context")
	}
	bot, err := check.Confirm(ctx, address, req.BotToken)
	if err != nil {
		return ChannelConnection{}, err
	}
	credentialRef, err := s.sealBotToken(ctx, ws, req.BotToken)
	if err != nil {
		return ChannelConnection{}, err
	}
	row, err := s.insertConnected(ctx, req.Provider, address, bot, credentialRef)
	if err != nil {
		// A lost race for a unique index GUARANTEES no row persisted, so the
		// just-sealed ref is safe to destroy. Any other error leaves the commit
		// outcome ambiguous, and deleting then could strand a live credential.
		if constraint, unique := storekit.UniqueViolation(err); unique {
			keyvault.DeleteDetached(ctx, s.vault, s.log, ws, credentialRef, "channel-connect-lost-race")
			return ChannelConnection{}, channelUniquenessRefusal(constraint)
		}
		return ChannelConnection{}, err
	}
	return row, nil
}

// channelUniquenessRefusal answers the ONE live-row uniqueness rule a connect or
// a replacement can lose to: uq_channel_connection_provider permits a single live
// binding per provider, so the remedy is always to disconnect what is bound —
// never to pick a different bot. The refusal says so, because "already
// connected" would send an admin looking for a binding of a bot they have never
// used.
//
// It still takes the constraint name rather than assuming: any OTHER unique
// index this table grows must not be answered as if it were this rule.
func channelUniquenessRefusal(constraint string) error {
	if constraint == channelOneLiveBotIndex {
		return fmt.Errorf("another bot is already connected to this installation; disconnect it first: %w",
			ErrChannelWorkspaceBotAlreadyBound)
	}
	return fmt.Errorf("this bot is already connected: %w", apperrors.ErrConflict)
}

// channelOneLiveBotIndex is the partial unique index that permits ONE live
// bot per installation (0151). Named here because the refusal above branches on
// it, and a rename in the migration must break this compile-time-invisible link
// loudly — which the connect suite's refusal assertion is what enforces.
//
// The index used to be named for a workspace column no table has. The Go name
// followed it, which is how a stale schema word reaches code that never touches
// the schema: it is the ONE live bot the index is about, not a tenant.
const channelOneLiveBotIndex = "uq_channel_connection_provider"

// requireConnectWiring refuses a connect this deployment cannot honestly
// complete: a provider with no composed check, or a missing vault (nothing could
// seal the token). Each refusal names what to fix.
func (s *ChannelStore) requireConnectWiring(provider string) (ChannelBotCheck, error) {
	check, err := s.botCheck(provider)
	if err != nil {
		return nil, err
	}
	if s.vault == nil {
		return nil, fmt.Errorf("configure a credential store for this installation, so a bot token can be sealed: %w",
			ErrChannelWiringIncomplete)
	}
	return check, nil
}

// connectAddress holds the provider's address rule: a self-hosted provider
// needs its install's web address, and a fixed-API one takes none.
func connectAddress(check ChannelBotCheck, raw string) (string, error) {
	address := strings.TrimRight(strings.TrimSpace(raw), "/")
	if !check.NeedsAddress() {
		if address != "" {
			return "", fmt.Errorf("this provider takes no web address: %w", apperrors.ErrInvalidArgument)
		}
		return "", nil
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("enter the chat's web address, such as https://chat.example.com: %w", apperrors.ErrInvalidArgument)
	}
	return address, nil
}

// sealBotToken puts the token in the vault before any row names it
// (put-then-commit). One secret per connection now: a poll authenticates with the
// bot token alone, so there is no second value to mint, rotate or destroy.
func (s *ChannelStore) sealBotToken(ctx context.Context, ws ids.UUID, token string) (keyvault.Ref, error) {
	ref, err := s.vault.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(token))
	if err != nil {
		return "", fmt.Errorf("capture: sealing the bot token: %w", err)
	}
	return ref, nil
}

// insertConnected writes the live row and its audit in one transaction. The
// cursor starts at 0 — "whatever Telegram still holds" — so a freshly connected
// bot collects the messages waiting for it rather than starting from silence.
func (s *ChannelStore) insertConnected(ctx context.Context, provider, address string, bot ChannelBot, credentialRef keyvault.Ref) (ChannelConnection, error) {
	connectedBy, err := channelActor(ctx)
	if err != nil {
		return ChannelConnection{}, err
	}
	var out ChannelConnection
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		out, err = scanChannelConnection(tx.QueryRow(ctx, `
			INSERT INTO channel_connection
			  (provider, channel_id, channel_label, address, credential_ref, status, poll_offset, connected_by)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, 0, $7)
			RETURNING `+channelConnectionColumns,
			provider, bot.ID, bot.Label, address,
			string(credentialRef), channelStatusConnected, connectedBy))
		if err != nil {
			return err
		}
		return auditLifecycle(ctx, tx, "create", channelConnectionObject, out.ID, nil, channelAuditImage(out))
	})
	if err != nil {
		return ChannelConnection{}, err
	}
	return out, nil
}

// channelActor resolves the human a connection is attributed to. connected_by
// is audit only — never an owner — but it is NOT NULL, so a principal with no
// human identity cannot connect a channel.
func channelActor(ctx context.Context) (ids.UUID, error) {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return ids.Nil, err
	}
	switch {
	case !p.UserID.IsZero():
		return p.UserID, nil
	case !p.OnBehalfOf.IsZero():
		return p.OnBehalfOf, nil
	default:
		return ids.Nil, fmt.Errorf("a channel connection records the human who made it: %w", apperrors.ErrPermissionDenied)
	}
}

// channelAuditImage is one side of a channel connection's audit trail. Neither
// vault ref appears: the audit spine must not become a second custodian of the
// credentials, and a ref tells a reader nothing the bot's own identity does not.
func channelAuditImage(c ChannelConnection) map[string]any {
	return map[string]any{
		"provider":      c.Provider,
		"channel_id":    c.ChannelID,
		"channel_label": c.ChannelLabel,
		"address":       c.Address,
		"status":        c.Status,
	}
}

func scanChannelConnection(r pgx.Row) (ChannelConnection, error) {
	var c ChannelConnection
	var address *string
	err := r.Scan(&c.ID, &c.Provider, &c.ChannelID, &c.ChannelLabel, &address,
		&c.Status, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	if address != nil {
		c.Address = *address
	}
	return c, err
}
