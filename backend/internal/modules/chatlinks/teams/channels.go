// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type team struct {
	ID   string `json:"id"`
	Name string `json:"displayName"`
}
type discovery struct {
	NextChats    string `json:"next_chats,omitempty"`
	Teams        []team
	NextTeams    string
	NextChannels string
}

// Channels lists native rooms available to the current credential.
func (c *Client) Channels(ctx context.Context, cursor string) ([]messaging.Conversation, string, error) {
	state, err := channelDiscovery(cursor)
	if err != nil {
		return nil, "", err
	}
	if state.NextChats != "" {
		rooms, next, err := c.chatChoices(ctx, state)
		if err != nil || len(rooms) > 0 || next.NextChats != "" {
			if err != nil {
				return nil, "", err
			}
			cursor, err := discoveryCursor(next)
			return rooms, cursor, err
		}
		state = next
	}
	return c.channelChoices(ctx, state)
}

func (c *Client) channelChoices(ctx context.Context, state discovery) ([]messaging.Conversation, string, error) {
	var err error
	state, err = c.discoverTeams(ctx, state)
	if err != nil {
		return nil, "", err
	}
	if len(state.Teams) == 0 {
		return []messaging.Conversation{}, "", nil
	}
	current := state.Teams[0]
	path := "/teams/" + url.PathEscape(current.ID) + "/channels"
	if state.NextChannels != "" {
		next, err := nextPath(state.NextChannels)
		if err != nil || !strings.HasPrefix(next, path+"?") {
			return nil, "", messaging.ErrInvalid
		}
		path = next
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var page struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"displayName"`
			Kind string `json:"membershipType"`
		} `json:"value"`
		Next string `json:"@odata.nextLink"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil, "", messaging.ErrUnavailable
	}
	out := []messaging.Conversation{}
	for _, ch := range page.Channels {
		out = append(out, messaging.Conversation{ID: current.ID + "/" + ch.ID, Name: current.Name + " / " + ch.Name, Kind: "channel", SupportsThreads: true})
	}
	state.NextChannels = page.Next
	if page.Next == "" {
		state.Teams = state.Teams[1:]
	}
	if len(state.Teams) == 0 && state.NextTeams == "" {
		return out, "", nil
	}
	next, err := discoveryCursor(state)
	return out, next, err
}

func channelDiscovery(cursor string) (discovery, error) {
	state := discovery{NextChats: "https://graph.microsoft.com/v1.0/me/chats?$top=50&$expand=members", NextTeams: "https://graph.microsoft.com/v1.0/me/joinedTeams"}
	if cursor != "" {
		if len(cursor) > 65536 {
			return discovery{}, &storekit.MalformedCursorError{}
		}
		decoded, err := storekit.DecodeOpaque[discovery](cursor)
		if err != nil {
			return discovery{}, err
		}
		if len(decoded.Teams) == 0 && decoded.NextTeams == "" && decoded.NextChats == "" {
			return discovery{}, &storekit.MalformedCursorError{}
		}
		state = decoded
	}
	return state, nil
}

func (c *Client) discoverTeams(ctx context.Context, state discovery) (discovery, error) {
	for range 20 {
		if len(state.Teams) > 0 {
			break
		}
		if state.NextTeams == "" {
			return state, nil
		}
		path, err := nextPath(state.NextTeams)
		if err != nil || !strings.HasPrefix(path, "/me/joinedTeams?") {
			return state, messaging.ErrInvalid
		}
		raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
		if err != nil {
			return state, err
		}
		//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
		var page struct {
			Teams []team `json:"value"`
			Next  string `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return state, messaging.ErrUnavailable
		}
		if page.Next != "" && page.Next == state.NextTeams {
			return state, messaging.ErrUnavailable
		}
		state.Teams = page.Teams
		state.NextTeams = page.Next
	}
	if len(state.Teams) == 0 {
		return state, messaging.ErrUnavailable
	}
	return state, nil
}

func discoveryCursor(state discovery) (string, error) {
	if state.NextChats == "" && len(state.Teams) == 0 && state.NextTeams == "" {
		return "", nil
	}
	return storekit.EncodeOpaque(state)
}

func (c *Client) chatChoices(ctx context.Context, state discovery) ([]messaging.Conversation, discovery, error) {
	path, err := nextPath(state.NextChats)
	if err != nil || !strings.HasPrefix(path, "/me/chats?") {
		return nil, state, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, state, err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var page struct {
		Items []struct {
			ID      string `json:"id"`
			Topic   string `json:"topic"`
			Kind    string `json:"chatType"`
			URL     string `json:"webUrl"`
			Members []struct {
				ID   string `json:"userId"`
				Name string `json:"displayName"`
			} `json:"members"`
		} `json:"value"`
		Next string `json:"@odata.nextLink"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Next != "" && page.Next == state.NextChats {
		return nil, state, messaging.ErrUnavailable
	}
	rooms := []messaging.Conversation{}
	for _, chat := range page.Items {
		if chat.ID == "" {
			return nil, state, messaging.ErrUnavailable
		}
		kind := groupChatKind
		if chat.Kind == directChatKind {
			kind = "direct"
		}
		name := chat.Topic
		if name == "" {
			names := []string{}
			for _, member := range chat.Members {
				if member.ID != c.user && member.Name != "" {
					names = append(names, member.Name)
				}
			}
			name = strings.Join(names, ", ")
		}
		if name == "" {
			name = chat.ID
		}
		rooms = append(rooms, messaging.Conversation{ID: chat.ID, Name: name, Kind: kind, SourceURL: chat.URL})
	}
	state.NextChats = page.Next
	return rooms, state, nil
}
