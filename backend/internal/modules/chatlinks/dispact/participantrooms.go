// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type participantRoom struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Kind      string `json:"type"`
	Archived  bool   `json:"is_archived"`
	Member    bool   `json:"is_member"`
	Threads   bool   `json:"allow_threads"`
}

func (c *Participant) room(room participantRoom) (messaging.Conversation, error) {
	if room.ID == "" || !participantID.MatchString(room.Slug) || room.Archived {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	if room.Workspace != c.workspace || !room.Member {
		return messaging.Conversation{}, messaging.ErrUnauthorized
	}
	kind := channelKind
	switch room.Kind {
	case "public", privateChannelKind:
	case "dm", "self_dm":
		kind = "direct"
	case "group_dm":
		kind = "group"
	default:
		return messaging.Conversation{}, messaging.ErrUnsupported
	}
	access := "participants"
	switch room.Kind {
	case "public":
		access = "workspace"
	case "private":
		access = "private"
	}
	return messaging.Conversation{Access: access, ID: room.ID, Name: room.Name, Kind: kind, SupportsThreads: room.Threads}, nil
}

func (c *Participant) channel(ctx context.Context, slug string) (participantRoom, error) {
	if !participantID.MatchString(slug) {
		return participantRoom{}, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodGet, "/channels/"+url.PathEscape(immutableChannelRef(slug))+"?reopen=false", nil)
	if err != nil {
		return participantRoom{}, err
	}
	var room participantRoom
	if json.Unmarshal(raw, &room) != nil {
		return participantRoom{}, messaging.ErrUnavailable
	}
	_, immutable := ids.Parse(slug)
	if immutable == nil && room.ID != slug || immutable != nil && room.Slug != slug {
		return participantRoom{}, messaging.ErrUnavailable
	}
	if _, err := c.room(room); err != nil {
		return participantRoom{}, err
	}
	return room, nil
}

// Conversation carries native room identity and audience-verification state.
func (c *Participant) Conversation(ctx context.Context, slug string) (messaging.Conversation, error) {
	room, err := c.channel(ctx, slug)
	if err != nil {
		return messaging.Conversation{}, err
	}
	out, err := c.room(room)
	if err != nil {
		return messaging.Conversation{}, err
	}
	cursor := ""
	seen := map[string]bool{}
	for range 10 {
		raw, err := c.http.Call(ctx, http.MethodGet, "/channels/"+url.PathEscape(immutableChannelRef(room.ID))+"/members-paginated?"+url.Values{limitKey: {"100"}, cursorKey: {cursor}}.Encode(), nil)
		if err != nil {
			return messaging.Conversation{}, err
		}
		var page struct {
			Members []struct {
				ID     string `json:"user_id"`
				Active bool   `json:"is_active"`
			} `json:"members"`
			Next  string `json:"next_cursor"`
			Total int    `json:"total"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return messaging.Conversation{}, messaging.ErrUnavailable
		}
		for _, row := range page.Members {
			if seen[row.ID] {
				return messaging.Conversation{}, messaging.ErrUnavailable
			}
			seen[row.ID] = true
			member, err := c.member(ctx, row.ID)
			if err != nil {
				return messaging.Conversation{}, err
			}
			member.Active = member.Active && row.Active
			out.Members = append(out.Members, member)
			out.External = out.External || member.External
		}
		if page.Next == "" {
			if len(out.Members) != page.Total {
				return messaging.Conversation{}, messaging.ErrUnavailable
			}
			out.AudienceKnown = true
			break
		}
		if cursor == page.Next {
			return messaging.Conversation{}, messaging.ErrUnavailable
		}
		cursor = page.Next
	}
	if !messaging.HasActiveMember(out.Members, c.user) {
		return messaging.Conversation{}, messaging.ErrUnauthorized
	}
	return out, nil
}

// Channels lists native rooms available to the current credential.
func (c *Participant) Channels(ctx context.Context, cursor string) ([]messaging.Conversation, string, error) {
	raw, err := c.http.Call(ctx, http.MethodGet, "/channels?"+url.Values{"limit": {"100"}, "cursor": {cursor}}.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	var page struct {
		Channels []participantRoom `json:"channels"`
		Next     string            `json:"next_cursor"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil, "", messaging.ErrUnavailable
	}
	rooms := []messaging.Conversation{}
	for _, row := range page.Channels {
		room, err := c.room(row)
		if err == nil && room.Kind == channelKind {
			rooms = append(rooms, room)
		}
	}
	return rooms, page.Next, nil
}

// OpenPeople opens the explicitly selected native recipient set.
func (c *Participant) OpenPeople(ctx context.Context, people []string) (messaging.Conversation, error) {
	if len(people) < 1 || len(people) > MaxGroupRecipients {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	seen, err := c.validatePeople(ctx, people)
	if err != nil {
		return messaging.Conversation{}, err
	}
	body, err := json.Marshal(struct {
		People []string `json:"user_ids"`
	}{people})
	if err != nil {
		return messaging.Conversation{}, err
	}
	raw, err := c.http.Call(ctx, http.MethodPost, "/dms", body)
	if err != nil {
		return messaging.Conversation{}, err
	}
	var opened participantRoom
	if json.Unmarshal(raw, &opened) != nil {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	room, err := c.Conversation(ctx, opened.ID)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if !room.AudienceKnown || room.External || room.Kind == channelKind || len(room.Members) != len(seen) {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	for _, member := range room.Members {
		if !seen[member.ID] || !member.Active {
			return messaging.Conversation{}, messaging.ErrInvalid
		}
	}
	return room, nil
}

func (c *Participant) validatePeople(ctx context.Context, people []string) (map[string]bool, error) {
	seen := map[string]bool{c.user: true}
	for _, id := range people {
		if seen[id] {
			return nil, messaging.ErrInvalid
		}
		seen[id] = true
		member, err := c.member(ctx, id)
		if err != nil {
			return nil, err
		}
		if !member.Active || member.External {
			return nil, messaging.ErrInvalid
		}
	}
	return seen, nil
}
