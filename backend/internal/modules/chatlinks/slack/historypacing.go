// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type historyPacing struct {
	profile *chatwire.HistoryProfile
	key     string
	meter   *ratelimit.Pacer
	sleep   func(context.Context, time.Duration) error
}

// WithHistoryProfile pins Slack history reads to the reviewed installation limits.
func (b *Bot) WithHistoryProfile(profile *chatwire.HistoryProfile, app string) *Bot {
	digest := sha256.Sum256([]byte(app + "\x00" + b.client.workspace))
	b.history = historyPacing{profile: profile, key: "slack/history/" + hex.EncodeToString(digest[:]), meter: ratelimit.NewPacer(), sleep: backoff.Sleep}
	return b
}

// WithHistoryPacing shares installation rate limits across recreated Slack clients.
func (b *Bot) WithHistoryPacing(meter *ratelimit.Pacer, sleep func(context.Context, time.Duration) error) *Bot {
	b.history.meter, b.history.sleep = meter, sleep
	return b
}

func (b *Bot) waitHistory(ctx context.Context, method string) error {
	if !b.history.profile.CanonicalSupported() || b.history.meter == nil || b.history.sleep == nil {
		return chatwire.ErrHistoryProfileNeedsReview
	}
	rate := b.history.profile.HistoryPerMinute
	if method == repliesMethod {
		rate = b.history.profile.RepliesPerMinute
	}
	for range 20 {
		wait, _, err := b.history.meter.Reserve(ctx, ratelimit.Budget{Key: b.history.key + "/" + method, PerMinute: rate, Cost: 1})
		if err != nil {
			return errors.Join(chatwire.ErrUnreachable, err)
		}
		if wait <= 0 {
			return nil
		}
		if wait > 15*time.Second {
			return &connector.RateLimitedError{RetryAfter: wait}
		}
		if err := b.history.sleep(ctx, wait); err != nil {
			return err
		}
	}
	return &connector.RateLimitedError{RetryAfter: time.Minute}
}

func (b *Bot) pauseHistory(ctx context.Context, method string, err error) error {
	var limited *connector.RateLimitedError
	if b.history.meter != nil && errors.As(err, &limited) && limited.RetryAfter > 0 {
		return errors.Join(err, b.history.meter.Pause(ctx, b.history.key+"/"+method, limited.RetryAfter))
	}
	return err
}
