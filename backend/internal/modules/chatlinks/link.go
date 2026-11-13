// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The link states a reader sees.
const (
	StateOff    = "off"
	StateNone   = "none"
	StateLinked = "linked"
	StatePaused = "paused"
)

// State is what a record's channel panel shows.
type State struct {
	State   string
	Link    *Link
	Address string
	Invites *Invites
}

// Invites says who a newly created channel was opened to.
type Invites struct {
	Invited, NotInChat, SkippedOverCap int
}

// LinkRequest is a link as the caller asked for it.
type LinkRequest struct {
	Mode             string
	Channel          string
	Verbosity        string
	SummarizeHistory *bool
}

// The link modes.
const (
	ModeCreate   = "create"
	ModeExisting = "existing"
)

// GetState reads the record's channel. The record must be visible to the
// caller; an unknown or hidden one answers ErrNotFound.
func (s *Store) GetState(ctx context.Context, kind string, id ids.UUID) (State, error) {
	if _, err := recordColumn(kind); err != nil {
		return State{}, err
	}
	if err := auth.Require(ctx, kind, principal.ActionRead); err != nil {
		return State{}, err
	}
	var out State
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		var err error
		out, err = stateOf(ctx, tx, kind, id)
		return err
	})
	return out, err
}

func stateOf(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (State, error) {
	var address *string
	var connected bool
	var connection ids.UUID
	err := tx.QueryRow(ctx, `SELECT id, address, status = 'connected' FROM channel_connection
		WHERE provider = $1 AND archived_at IS NULL`, ProviderDispact).Scan(&connection, &address, &connected)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{State: StateOff}, nil
	}
	if err != nil {
		return State{}, err
	}
	out := State{State: StateNone}
	if address != nil {
		out.Address = *address
	}
	link, err := liveLinkOf(ctx, tx, kind, id)
	if errors.Is(err, ErrLinkNotFound) {
		if !connected {
			out.State = StateOff
		}
		return out, nil
	}
	if err != nil {
		return State{}, err
	}
	out.Link = &link
	out.State = StateLinked
	if !connected || link.Status != StatusActive || link.ConnectionID != connection {
		out.State = StatePaused
	}
	return out, nil
}

// Link links a record to a channel the bot creates or to a private channel it
// is already in. The record is checked before and again after the chat is
// called, because no transaction is held across the call.
func (s *Store) Link(ctx context.Context, kind string, id ids.UUID, req LinkRequest) (State, error) {
	if err := auth.Require(ctx, kind, principal.ActionUpdate); err != nil {
		return State{}, err
	}
	if err := validateLink(&req); err != nil {
		return State{}, err
	}
	facts, err := s.precheckLink(ctx, kind, id)
	if err != nil {
		return State{}, err
	}
	session, err := s.chats.Open(ctx)
	if err != nil {
		return State{}, err
	}
	channel, err := resolveChannel(ctx, session.Chat, kind, id, facts.name, req)
	if err != nil {
		return State{}, err
	}
	var invites *Invites
	if req.Mode == ModeCreate {
		invites, err = s.invite(ctx, session.Chat, channel.Slug, facts)
		if err != nil {
			return State{}, err
		}
	}
	link, err := s.insertLink(ctx, kind, id, session, channel, req)
	if err != nil {
		return State{}, err
	}
	return State{State: StateLinked, Link: &link, Address: session.Address, Invites: invites}, nil
}

func validateLink(req *LinkRequest) error {
	switch req.Mode {
	case ModeCreate:
		req.Channel = ""
		req.SummarizeHistory = nil
	case ModeExisting:
		req.Channel = channelSlugOf(req.Channel)
		if req.Channel == "" {
			return fmt.Errorf("name the channel by its slug or its link: %w", apperrors.ErrInvalidArgument)
		}
		if req.SummarizeHistory == nil {
			return fmt.Errorf("say whether to summarise the talk so far: %w", apperrors.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("mode is create or existing: %w", apperrors.ErrInvalidArgument)
	}
	if req.Verbosity == "" {
		req.Verbosity = VerbosityImportant
	}
	return validVerbosity(req.Verbosity)
}

func validVerbosity(v string) error {
	switch v {
	case VerbosityEverything, VerbosityImportant, VerbosityMilestones, VerbositySilent:
		return nil
	}
	return fmt.Errorf("verbosity is everything, important, milestones or silent: %w", apperrors.ErrInvalidArgument)
}

// channelSlugOf takes a slug, a #slug, or a channel link, and returns the slug.
func channelSlugOf(raw string) string {
	value := strings.TrimSpace(raw)
	if i := strings.IndexAny(value, "?#"); i > 0 {
		value = value[:i]
	}
	value = strings.TrimRight(value, "/")
	if i := strings.LastIndex(value, "/"); i >= 0 {
		value = value[i+1:]
	}
	return strings.ToLower(strings.TrimPrefix(value, "#"))
}

// recordFacts is what linking needs to know about the record.
type recordFacts struct {
	name       string
	ownerEmail string
	actorEmail string
	actor      ids.UUID
	projectID  *ids.UUID
}

// precheckLink refuses early what the write would refuse anyway. So the chat
// is never asked for a channel the record cannot take.
func (s *Store) precheckLink(ctx context.Context, kind string, id ids.UUID) (recordFacts, error) {
	actor, err := linkActor(ctx)
	if err != nil {
		return recordFacts{}, err
	}
	facts := recordFacts{actor: actor}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, kind, id); err != nil {
			return err
		}
		if _, err := liveLinkOf(ctx, tx, kind, id); err == nil {
			return ErrRecordAlreadyLinked
		} else if !errors.Is(err, ErrLinkNotFound) {
			return err
		}
		return readFacts(ctx, tx, kind, id, &facts)
	})
	return facts, err
}

func readFacts(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, facts *recordFacts) error {
	project := "NULL::uuid"
	if kind == RecordDeal {
		project = "r.project_id"
	}
	query := fmt.Sprintf(`SELECT r.name, coalesce(o.email, ''), %s,
		(SELECT coalesce(email, '') FROM app_user WHERE id = $2)
		FROM %s r LEFT JOIN app_user o ON o.id = r.owner_id WHERE r.id = $1`,
		project, pgx.Identifier{kind}.Sanitize())
	return tx.QueryRow(ctx, query, id, facts.actor).Scan(&facts.name, &facts.ownerEmail, &facts.projectID, &facts.actorEmail)
}

// linkActor is the human a link is recorded against: the signed-in member, or
// the member an agent acts for.
func linkActor(ctx context.Context) (ids.UUID, error) {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return ids.Nil, err
	}
	switch {
	case !p.UserID.IsZero():
		return p.UserID, nil
	case !p.OnBehalfOf.IsZero():
		return p.OnBehalfOf, nil
	}
	return ids.Nil, fmt.Errorf("a channel link records the member who made it: %w", apperrors.ErrPermissionDenied)
}

// resolveChannel creates the record's channel, or checks the named one: it
// must exist, be private, and have the bot in it.
func resolveChannel(ctx context.Context, chat Chat, kind string, id ids.UUID, name string, req LinkRequest) (dispact.Channel, error) {
	if req.Mode == ModeCreate {
		ch, err := chat.CreateChannel(ctx, externalKey(kind, id), channelNameOf(name))
		return ch, chatFailure(err)
	}
	ch, err := chat.Channel(ctx, req.Channel)
	if err != nil {
		return dispact.Channel{}, chatFailure(err)
	}
	switch {
	case ch.IsArchived:
		return dispact.Channel{}, ErrChannelNotFound
	case !ch.IsPrivate:
		return dispact.Channel{}, ErrChannelNotPrivate
	case !ch.BotIsMember:
		return dispact.Channel{}, ErrBotNotMember
	}
	return ch, nil
}

// externalKey names the channel Margince made for a record, so a retried
// create finds it instead of making a second one.
func externalKey(kind string, id ids.UUID) string {
	return "margince:" + kind + ":" + id.String()
}

// channelNameOf keeps a record's name within the chat's 80-character limit.
func channelNameOf(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}

// chatFailure turns the chat client's errors into this package's refusals.
func chatFailure(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, dispact.ErrChannelGone):
		return fmt.Errorf("%w: %w", ErrChannelNotFound, err)
	case errors.Is(err, dispact.ErrNotMember):
		return fmt.Errorf("%w: %w", ErrBotNotMember, err)
	}
	return fmt.Errorf("%w: %w", ErrChatUnreachable, err)
}

// invite opens a new channel to the member linking and the record's owner. A
// member with no chat account is counted, not refused.
func (s *Store) invite(ctx context.Context, chat Chat, slug string, facts recordFacts) (*Invites, error) {
	emails := uniqueEmails(facts.actorEmail, facts.ownerEmail)
	if len(emails) == 0 {
		return &Invites{}, nil
	}
	result, err := chat.AddMembers(ctx, slug, emails)
	if err != nil {
		return nil, chatFailure(err)
	}
	return &Invites{Invited: len(result.Added), NotInChat: len(result.NotFound)}, nil
}

func uniqueEmails(candidates ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, email := range candidates {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
