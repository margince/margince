// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	Invited, NotInChat, NotAdded, SkippedOverCap int
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
		invites, err = s.invite(ctx, session.Chat, channel.Slug, kind, id, facts)
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
	name      string
	owner     *ids.UUID
	actor     ids.UUID
	projectID *ids.UUID
}

// LinkableChannels lists the private channels the bot is in that no live link
// holds, sorted by slug, for a member who may link this record.
func (s *Store) LinkableChannels(ctx context.Context, kind string, id ids.UUID) ([]dispact.Channel, error) {
	if _, err := s.precheckLink(ctx, kind, id); err != nil {
		return nil, err
	}
	session, err := s.chats.Open(ctx)
	if err != nil {
		return nil, err
	}
	channels, err := session.Chat.Channels(ctx)
	if err != nil {
		return nil, ChatFailure(err)
	}
	linked := map[string]bool{}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT channel_slug FROM chat_link
			WHERE connection_id = $1 AND archived_at IS NULL`, session.ConnectionID)
		if err != nil {
			return err
		}
		slugs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		for _, slug := range slugs {
			linked[slug] = true
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]dispact.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.IsPrivate && ch.BotIsMember && !ch.IsArchived && !linked[ch.Slug] {
			out = append(out, ch)
		}
	}
	slices.SortFunc(out, func(a, b dispact.Channel) int { return strings.Compare(a.Slug, b.Slug) })
	return out, nil
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
	query := fmt.Sprintf(`SELECT r.name, r.owner_id, %s FROM %s r WHERE r.id = $1`,
		project, pgx.Identifier{kind}.Sanitize())
	return tx.QueryRow(ctx, query, id).Scan(&facts.name, &facts.owner, &facts.projectID)
}

// memberEmail is the address of the member asking, or of the member an agent
// acts for. Only the manifest reads it, after its own auth.Require.
func (s *Store) memberEmail(ctx context.Context) (string, error) {
	actor, err := linkActor(ctx)
	if err != nil {
		return "", err
	}
	var email string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(email, '') FROM app_user WHERE id = $1`, actor).Scan(&email)
	})
	return email, err
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
		return ch, ChatFailure(err)
	}
	ch, err := chat.Channel(ctx, req.Channel)
	if err != nil {
		return dispact.Channel{}, ChatFailure(err)
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

// ChatFailure turns the chat client's errors into this package's refusals.
func ChatFailure(err error) error {
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

// inviteCap is the most members a new channel is opened to.
const inviteCap = 50

// invite opens a new channel to the members who may open the record, the
// record's owner and the member linking first, at most inviteCap. A member
// with no chat account is counted, not refused.
func (s *Store) invite(ctx context.Context, chat Chat, slug, kind string, id ids.UUID, facts recordFacts) (*Invites, error) {
	var viewers []Viewer
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		viewers, err = s.viewers(ctx, tx, kind, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	emails, skipped := invitees(viewers, facts.owner, facts.actor)
	if len(emails) == 0 {
		return &Invites{}, nil
	}
	result, err := chat.AddMembers(ctx, slug, emails)
	if err != nil {
		return nil, ChatFailure(err)
	}
	return &Invites{
		Invited:   len(result.Added) + len(result.AlreadyMember),
		NotInChat: len(result.NotInChat), NotAdded: len(result.NotAdded), SkippedOverCap: skipped,
	}, nil
}

// invitees orders the viewers owner first, then the linker, then as given,
// and keeps inviteCap of them. It answers the emails kept and how many were
// left over.
func invitees(viewers []Viewer, owner *ids.UUID, actor ids.UUID) ([]string, int) {
	rank := func(v Viewer) int {
		switch {
		case owner != nil && v.ID == *owner:
			return 0
		case v.ID == actor:
			return 1
		}
		return 2
	}
	ordered := slices.Clone(viewers)
	slices.SortStableFunc(ordered, func(a, b Viewer) int { return rank(a) - rank(b) })
	emails := make([]string, 0, min(len(ordered), inviteCap))
	seen := map[string]bool{}
	skipped := 0
	for _, v := range ordered {
		key := strings.ToLower(strings.TrimSpace(v.Email))
		switch {
		case key == "" || seen[key]:
		case len(emails) == inviteCap:
			skipped++
		default:
			seen[key] = true
			emails = append(emails, key)
		}
	}
	return emails, skipped
}
