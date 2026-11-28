// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func (b *Bot) findWatch(ctx context.Context, in WatchRequest, resource string) (Subscription, error) {
	if in.Subscription != "" {
		current, err := b.readWatch(ctx, in, resource)
		if !errors.Is(err, messaging.ErrNotFound) {
			return current, err
		}
	}
	path := "/subscriptions"
	var found Subscription
	seen := map[string]bool{}
	for range 20 {
		if seen[path] {
			return Subscription{}, messaging.ErrUnavailable
		}
		seen[path] = true
		raw, err := b.graph.http.Call(ctx, http.MethodGet, path, nil)
		if err != nil {
			return Subscription{}, err
		}
		//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
		var page struct {
			Items []Subscription `json:"value"`
			Next  string         `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return Subscription{}, messaging.ErrUnavailable
		}
		for _, sub := range page.Items {
			if !watchMatches(sub, in, resource) {
				continue
			}
			if found.ID != "" || sub.ID == "" {
				return Subscription{}, messaging.ErrUnavailable
			}
			found = sub
		}
		if page.Next == "" {
			return found, nil
		}
		path, err = nextPath(page.Next)
		if err != nil || !strings.HasPrefix(path, "/subscriptions?") {
			return Subscription{}, messaging.ErrUnavailable
		}
	}
	return Subscription{}, messaging.ErrUnavailable
}

// RemoveWatch deletes only a subscription matching the stored channel binding.
func (b *Bot) RemoveWatch(ctx context.Context, in WatchRequest) error {
	resource, ok := b.watchResource(in.Channel)
	if !ok {
		return messaging.ErrInvalid
	}
	current, err := b.findWatch(ctx, in, resource)
	if err != nil {
		return err
	}
	if current.ID == "" {
		return nil
	}
	_, err = b.graph.http.Call(ctx, http.MethodDelete, "/subscriptions/"+url.PathEscape(current.ID), nil)
	if errors.Is(err, messaging.ErrNotFound) {
		return nil
	}
	return err
}

func (b *Bot) readWatch(ctx context.Context, in WatchRequest, resource string) (Subscription, error) {
	raw, err := b.graph.http.Call(ctx, http.MethodGet, "/subscriptions/"+url.PathEscape(in.Subscription), nil)
	if err == nil {
		var current Subscription
		if json.Unmarshal(raw, &current) != nil || current.ID != in.Subscription || !watchMatches(current, in, resource) {
			return Subscription{}, messaging.ErrUnauthorized
		}
		return current, nil
	}
	return Subscription{}, err
}
