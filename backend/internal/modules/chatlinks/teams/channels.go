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
		if ch.Kind == "shared" {
			continue
		}
		out = append(out, messaging.Conversation{ID: current.ID + "/" + ch.ID, Name: current.Name + " / " + ch.Name, Kind: "channel", SupportsThreads: true})
	}
	state.NextChannels = page.Next
	if page.Next == "" {
		state.Teams = state.Teams[1:]
	}
	if len(state.Teams) == 0 && state.NextTeams == "" {
		return out, "", nil
	}
	next, err := storekit.EncodeOpaque(state)
	return out, next, err
}

func channelDiscovery(cursor string) (discovery, error) {
	state := discovery{NextTeams: "https://graph.microsoft.com/v1.0/me/joinedTeams"}
	if cursor != "" {
		if len(cursor) > 65536 {
			return discovery{}, &storekit.MalformedCursorError{}
		}
		decoded, err := storekit.DecodeOpaque[discovery](cursor)
		if err != nil {
			return discovery{}, err
		}
		if len(decoded.Teams) == 0 && decoded.NextTeams == "" {
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
