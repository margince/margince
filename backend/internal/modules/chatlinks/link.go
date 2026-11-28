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

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	LastSync  *EndedSync
	Health    *SyncHealth
	Workspace string
	State     string
	Link      *Link
	Address   string
	Invites   *Invites
}

// Invites says who a newly created channel was opened to.
type Invites struct {
	Invited, NotInChat, NotAdded, SkippedOverCap int
}

// LinkRequest is a link as the caller asked for it.
type LinkRequest struct {
	participant      *MemberGrant
	RootID           string
	Provider         string
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
	if err := requireRecordRead(ctx, kind); err != nil {
		return State{}, err
	}
	var out State
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		var err error
		out, err = stateOf(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		out.LastSync, err = lastEndedSync(ctx, tx, kind, id)
		return err
	})
	return out, err
}

func stateOf(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (State, error) {
	link, err := liveLinkOf(ctx, tx, kind, id)
	if errors.Is(err, ErrLinkNotFound) {
		var connected bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channel_connection
   WHERE provider=ANY(@providers) AND status='connected' AND archived_at IS NULL
   AND account_id IS NULL AND parent_connection_id IS NULL)`, pgx.StrictNamedArgs{"providers": []string{ProviderDispact, ProviderSlack, ProviderTeams}}).Scan(&connected)
		if err != nil {
			return State{}, err
		}
		if connected {
			return State{State: StateNone}, nil
		}
		return State{State: StateOff}, nil
	}
	if err != nil {
		return State{}, err
	}
	var address *string
	var connected bool
	var workspace string
	err = tx.QueryRow(ctx, `SELECT channel_id,address,status='connected' AND archived_at IS NULL FROM channel_connection
  WHERE id=@id AND provider=@provider AND account_id IS NULL AND parent_connection_id IS NULL`, pgx.StrictNamedArgs{"id": link.ConnectionID, providerKey: link.Provider}).Scan(&workspace, &address, &connected)
	if errors.Is(err, pgx.ErrNoRows) {
		connected = false
	} else if err != nil {
		return State{}, err
	}
	health, err := linkSyncHealth(ctx, tx, link.ID)
	if err != nil {
		return State{}, err
	}
	out := State{State: StateLinked, Link: &link, Workspace: workspace, Health: &health}
	if address != nil {
		out.Address = *address
	}
	if !connected || link.Status != StatusActive {
		out.State = StatePaused
	}
	return out, nil
}

// Link links a record to a channel the bot creates or to a private channel it
// is already in. The record is checked before and again after the chat is
// called, because no transaction is held across the call.
func (s *Store) Link(ctx context.Context, kind string, id ids.UUID, req LinkRequest) (State, error) {
	if err := validateLink(&req); err != nil {
		return State{}, err
	}
	facts, err := s.precheckLink(ctx, kind, id)
	if err != nil {
		return State{}, err
	}
	session, err := OpenProvider(ctx, s.chats, req.Provider)
	if err != nil {
		return State{}, err
	}
	var participant messaging.Client
	_, canonical := session.Chat.(chatwire.RootReader)
	if req.Provider != ProviderDispact || canonical {
		grant, client, err := s.OpenMember(ctx, req.Provider)
		if err != nil {
			return State{}, err
		}
		if grant.Workspace != session.TeamID {
			return State{}, messaging.ErrUnauthorized
		}
		req.participant = &grant
		participant = client
	}
	channel, err := resolveChannel(ctx, session.Chat, kind, id, facts.name, req)
	if err != nil {
		return State{}, err
	}
	if req.RootID != "" {
		reader, ok := session.Chat.(chatwire.RootReader)
		if !ok || req.Mode != ModeExisting {
			return State{}, apperrors.ErrInvalidArgument
		}
		if _, err := reader.Thread(ctx, channel.Slug, req.RootID, ""); err != nil {
			return State{}, ChatFailure(err)
		}
	}
	var invites *Invites
	if req.Mode == ModeCreate {
		invites, err = s.invite(ctx, session.Chat, channel.Slug, kind, id, facts)
		if err != nil {
			return State{}, err
		}
	}
	if participant != nil {
		if _, err := participant.Conversation(ctx, channel.Slug); err != nil {
			return State{}, err
		}
	}

	link, err := s.insertLink(ctx, kind, id, session, channel, req)
	if err != nil {
		return State{}, err
	}
	return State{State: StateLinked, Link: &link, Address: session.Address, Workspace: session.TeamID, Invites: invites}, nil
}

func validateLink(req *LinkRequest) error {
	if req.Provider == "" {
		req.Provider = ProviderDispact
	}
	if !IsChatProvider(req.Provider) {
		return fmt.Errorf("choose a supported chat provider: %w", apperrors.ErrInvalidArgument)
	}
	switch req.Mode {
	case ModeCreate:
		req.Channel = ""
		req.SummarizeHistory = nil
	case ModeExisting:
		if req.Provider == ProviderDispact {
			req.Channel = channelSlugOf(req.Channel)
		} else {
			req.Channel = strings.TrimSpace(req.Channel)
		}
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
func (s *Store) LinkableChannels(ctx context.Context, kind string, id ids.UUID) ([]chatwire.Channel, error) {
	return s.LinkableProviderChannels(ctx, kind, id, ProviderDispact)
}

// LinkableProviderChannels limits discovery to destinations the record may associate.
func (s *Store) LinkableProviderChannels(ctx context.Context, kind string, id ids.UUID, provider string) ([]chatwire.Channel, error) {
	if _, err := s.precheckLink(ctx, kind, id); err != nil {
		return nil, err
	}
	session, err := OpenProvider(ctx, s.chats, provider)
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
	out := make([]chatwire.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.IsPrivate && ch.BotIsMember && !ch.IsArchived && !linked[ch.Slug] {
			out = append(out, ch)
		}
	}
	slices.SortFunc(out, func(a, b chatwire.Channel) int { return strings.Compare(a.Slug, b.Slug) })
	return out, nil
}

// precheckLink refuses early what the write would refuse anyway. So the chat
// is never asked for a channel the record cannot take.
func (s *Store) precheckLink(ctx context.Context, kind string, id ids.UUID) (recordFacts, error) {
	if err := requireRecordUpdate(ctx, kind); err != nil {
		return recordFacts{}, err
	}
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
	name := "r.name"
	if kind == RecordCompany {
		name = "r.display_name"
	}
	query := fmt.Sprintf(`SELECT %s, r.owner_id, %s FROM %s r WHERE r.id = $1`,
		name,
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
func resolveChannel(ctx context.Context, chat Chat, kind string, id ids.UUID, name string, req LinkRequest) (chatwire.Channel, error) {
	if req.Mode == ModeCreate {
		return createChannel(ctx, chat, kind, id, name)
	}
	ch, err := LiveChannel(ctx, chat, req.Channel)
	if err != nil {
		return chatwire.Channel{}, ChatFailure(err)
	}
	switch {
	case !ch.IsPrivate:
		return chatwire.Channel{}, ErrChannelNotPrivate
	case !ch.BotIsMember:
		return chatwire.Channel{}, ErrBotNotMember
	}
	return ch, nil
}

// LiveChannel reads a channel and answers an archived one as gone. The chat
// still serves an archived channel's history and takes the bot's posts.
func LiveChannel(ctx context.Context, chat Chat, slug string) (chatwire.Channel, error) {
	ch, err := chat.Channel(ctx, slug)
	if err == nil && ch.IsArchived {
		err = fmt.Errorf("chatlinks: channel %s is archived: %w", slug, dispact.ErrChannelGone)
	}
	return ch, err
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
	case errors.Is(err, dispact.ErrNameRefused):
		return fmt.Errorf("%w: %w", ErrChannelNameRefused, err)
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
		key := correspondence.Fold(v.Email)
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
