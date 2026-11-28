// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// WatchRequest pins subscription operations to one callback and channel binding.
type WatchRequest struct {
	Channel      string
	URL          string
	Secret       string
	Subscription string
	Reauthorize  bool
	Now          time.Time
}

// WatchMaxLifetime bounds leases so expired subscriptions can be retired safely.
const WatchMaxLifetime = 72 * time.Hour

// Subscription retains the provider lease identity and expiry needed for renewal.
//
//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type Subscription struct {
	ID              string    `json:"id,omitempty"`
	Resource        string    `json:"resource"`
	NotificationURL string    `json:"notificationUrl"`
	LifecycleURL    string    `json:"lifecycleNotificationUrl"`
	ClientState     string    `json:"clientState"`
	ChangeType      string    `json:"changeType"`
	ResourceData    bool      `json:"includeResourceData"`
	Expires         time.Time `json:"expirationDateTime"`
}

// EnsureWatch creates or renews a subscription bound to the reviewed channel.
func (b *Bot) EnsureWatch(ctx context.Context, in WatchRequest) (Subscription, error) {
	resource, ok := b.watchResource(in.Channel)
	if !ok || in.Now.IsZero() || !ValidWatchURL(in.URL) || len(in.Secret) < 32 || len(in.Secret) > 128 {
		return Subscription{}, messaging.ErrInvalid
	}
	current, err := b.findWatch(ctx, in, resource)
	if err != nil {
		return Subscription{}, err
	}
	if current.ID != "" {
		return b.renewWatch(ctx, in, current)
	}
	desired := Subscription{Resource: resource, NotificationURL: in.URL, LifecycleURL: in.URL, ClientState: in.Secret, ChangeType: "created,updated,deleted", Expires: in.Now.Add(48 * time.Hour)}
	raw, err := json.Marshal(desired)
	if err != nil {
		return Subscription{}, err
	}
	response, err := b.graph.http.Call(ctx, http.MethodPost, "/subscriptions", raw)
	if err != nil {
		return Subscription{}, err
	}
	var created Subscription
	if json.Unmarshal(response, &created) != nil || !watchMatches(created, in, resource) || created.ID == "" || !validWatchExpiry(created.Expires, in.Now) {
		return Subscription{}, messaging.ErrUnavailable
	}
	return created, nil
}

func (b *Bot) watchResource(channel string) (string, bool) {
	path, ok := channelPath(channel)
	team, _, _ := strings.Cut(channel, "/")
	return path + "/messages", ok && team == b.team
}

// ValidWatchURL restricts subscriptions to supported callback origins.
func ValidWatchURL(address string) bool {
	parsed, err := url.Parse(address)
	return len(address) <= 2048 && err == nil && parsed.Scheme == httpsScheme && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && parsed.RawQuery == ""
}

func watchMatches(sub Subscription, in WatchRequest, resource string) bool {
	decoded, err := url.PathUnescape(sub.Resource)
	expected, expectedErr := url.PathUnescape(resource)
	return err == nil && expectedErr == nil && strings.TrimPrefix(decoded, "/") == strings.TrimPrefix(expected, "/") && sub.NotificationURL == in.URL && sub.LifecycleURL == in.URL && !sub.ResourceData && watchChanges(sub.ChangeType) && subtle.ConstantTimeCompare([]byte(sub.ClientState), []byte(in.Secret)) == 1
}

func watchChanges(value string) bool {
	remaining := map[string]bool{"created": true, "updated": true, "deleted": true}
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if !remaining[part] {
			return false
		}
		delete(remaining, part)
	}
	return len(remaining) == 0
}

// WatchBinding returns the application and tenant that own subscription operations.
func (b *Bot) WatchBinding() (string, string) {
	return b.app, b.graph.tenant
}

func validWatchExpiry(expires, now time.Time) bool {
	return expires.After(now) && !expires.After(now.Add(WatchMaxLifetime))
}

func (b *Bot) renewWatch(ctx context.Context, in WatchRequest, current Subscription) (Subscription, error) {
	if in.Reauthorize {
		if _, err := b.graph.http.Call(ctx, http.MethodPost, "/subscriptions/"+url.PathEscape(current.ID)+"/reauthorize", nil); err != nil {
			return Subscription{}, err
		}
	}
	if !in.Reauthorize && current.Expires.After(in.Now.Add(12*time.Hour)) {
		return current, nil
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	raw, err := json.Marshal(struct {
		Expires time.Time `json:"expirationDateTime"`
	}{in.Now.Add(48 * time.Hour)})
	if err != nil {
		return Subscription{}, err
	}
	updated, err := b.graph.http.Call(ctx, http.MethodPatch, "/subscriptions/"+url.PathEscape(current.ID), raw)
	if err != nil {
		return Subscription{}, err
	}
	var renewal Subscription
	if json.Unmarshal(updated, &renewal) != nil || renewal.ID != current.ID || !validWatchExpiry(renewal.Expires, in.Now) {
		return Subscription{}, messaging.ErrUnavailable
	}
	current.Expires = renewal.Expires
	return current, nil
}
