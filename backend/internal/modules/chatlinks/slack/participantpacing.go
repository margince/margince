// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
)

// WithParticipantHistory shares app/workspace quotas with channel reads; unknown installations use the reduced tier.
func (c *Client) WithParticipantHistory(profile *chatwire.HistoryProfile, app string) *Client {
	if profile == nil {
		profile = &chatwire.HistoryProfile{HistoryPerMinute: 1, RepliesPerMinute: 1, HistoryPageSize: 15, RepliesPageSize: 15}
	}
	digest := sha256.Sum256([]byte(app + "\x00" + c.workspace))
	c.history = &historyPacing{profile: profile, key: "slack/history/" + hex.EncodeToString(digest[:]), meter: ratelimit.NewPacer(), sleep: backoff.Sleep}
	return c
}

func (c *Client) waitParticipantHistory(ctx context.Context, method string) error {
	if c.history == nil {
		return nil
	}
	return c.history.wait(ctx, method)
}

func (c *Client) pauseParticipantHistory(ctx context.Context, path string, failure error) error {
	method, _, _ := strings.Cut(path, "?")
	if c.history != nil && (method == repliesMethod || method == historyMethod) {
		return c.history.pause(ctx, method, failure)
	}
	return failure
}
