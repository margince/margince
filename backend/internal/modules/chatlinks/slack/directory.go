// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func (c *Client) directoryPage(ctx context.Context, cursor string) ([]user, string, error) {
	raw, err := c.call(ctx, http.MethodGet, "users.list?"+url.Values{limitKey: {"200"}, cursorKey: {cursor}}.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	var page struct {
		Members  []user `json:"members"`
		Metadata struct {
			Next string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Metadata.Next != "" && page.Metadata.Next == cursor {
		return nil, "", messaging.ErrUnavailable
	}
	return page.Members, page.Metadata.Next, nil
}

// People searches native identities through the current participant credential.
func (c *Client) People(ctx context.Context, query, cursor string) ([]messaging.Member, string, error) {
	seen := map[string]bool{}
	for range 100 {
		people, next, err := c.directoryPage(ctx, cursor)
		if err != nil {
			return nil, "", err
		}
		result := []messaging.Member{}
		for _, u := range people {
			member := c.person(u)
			if member.ID != "" && member.Active && !member.External && !u.Bot && strings.Contains(strings.ToLower(member.Name+" "+member.Email), strings.ToLower(query)) {
				result = append(result, member)
			}
		}
		if len(result) > 0 || next == "" {
			return result, next, nil
		}
		if seen[next] {
			return nil, "", messaging.ErrUnavailable
		}
		seen[next] = true
		cursor = next
	}
	return nil, "", messaging.ErrUnavailable
}

func (c *Client) rosterMembers(ctx context.Context, identities []string) ([]messaging.Member, error) {
	members := map[string]messaging.Member{}
	for _, id := range identities {
		if id == "" {
			return nil, messaging.ErrUnavailable
		}
		if _, exists := members[id]; exists {
			return nil, messaging.ErrUnavailable
		}
		members[id] = messaging.Member{}
	}
	if len(identities) < 10 {
		for _, id := range identities {
			member, err := c.member(ctx, id)
			if err != nil {
				return nil, err
			}
			members[id] = member
		}
	} else {
		if err := c.fillRoster(ctx, members, len(identities)); err != nil {
			return nil, err
		}
	}
	out := make([]messaging.Member, 0, len(identities))
	for _, id := range identities {
		member := members[id]
		if member.ID == "" {
			return nil, messaging.ErrUnavailable
		}
		out = append(out, member)
	}
	return out, nil
}

func (c *Client) fillRoster(ctx context.Context, members map[string]messaging.Member, remaining int) error {
	cursor := ""
	seen := map[string]bool{}
	for range 100 {
		page, next, err := c.directoryPage(ctx, cursor)
		if err != nil {
			return err
		}
		for _, u := range page {
			if old, wanted := members[u.ID]; wanted && old.ID == "" {
				members[u.ID] = c.person(u)
				remaining--
			}
		}
		if remaining == 0 {
			break
		}
		if next == "" || seen[next] {
			return messaging.ErrUnavailable
		}
		seen[next] = true
		cursor = next
	}
	return nil
}
