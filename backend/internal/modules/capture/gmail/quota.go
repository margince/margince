// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	defaultMailboxQuota = 6000
	defaultProjectQuota = 1200000
)

type quotaOwnerKey struct{}

type quotaYieldError struct{ limit *connector.RateLimitedError }

func (e *quotaYieldError) Error() string { return e.limit.Error() }
func (e *quotaYieldError) Unwrap() error { return e.limit }

type quotaCoordinator interface {
	Reserve(context.Context, ...ratelimit.Budget) (time.Duration, string, error)
	Pause(context.Context, string, time.Duration) error
}

type quotaPacing struct {
	meter            quotaCoordinator
	mailbox, project int
	sleep            func(context.Context, time.Duration) error
	now              func() time.Time
}

func quotaOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, quotaOwnerKey{}, strings.ToLower(strings.TrimSpace(owner)))
}

func (c *Connector) authorize(ctx context.Context, st authState) (context.Context, string, error) {
	ctx = quotaOwner(ctx, st.Owner)
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	return ctx, access, err
}

func newQuotaPacing(mailbox, project int) quotaPacing {
	if mailbox == 0 {
		mailbox = defaultMailboxQuota
	}
	if project == 0 {
		project = defaultProjectQuota
	}
	return quotaPacing{meter: ratelimit.NewPacer(), mailbox: mailbox, project: project, sleep: backoff.Sleep, now: time.Now}
}

func (a *httpAPI) waitQuota(ctx context.Context, cost int) error {
	// Leave room for one largest call at a rolling-window boundary.
	budgets := []ratelimit.Budget{{Key: "gmail/project", PerMinute: a.quota.project - 100, Cost: cost}}
	if owner, ok := ctx.Value(quotaOwnerKey{}).(string); ok && owner != "" {
		budgets = append(budgets, ratelimit.Budget{Key: mailboxQuotaKey(owner), PerMinute: a.quota.mailbox - 100, Cost: cost})
	}
	started := a.quota.now()
	for {
		wait, key, err := a.quota.meter.Reserve(ctx, budgets...)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("gmail: quota coordination unavailable: %w", ErrUnreachable)
		}
		if wait <= 0 {
			return nil
		}
		if a.quota.now().Sub(started)+wait > rateWaitInPage {
			reason := connector.RateLimitRate
			if strings.Contains(key, "gmail/mailbox/") {
				reason = connector.RateLimitUser
			}
			return &quotaYieldError{limit: &connector.RateLimitedError{RetryAfter: wait, Reason: reason}}
		}
		capturemetrics.ObservePacing(ctx, wait)
		if err := a.quota.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

func getQuotaCost(path string, q url.Values) int {
	switch {
	case path == "/profile", path == "/labels":
		return 1
	case path == "/history":
		return 2
	case path == "/messages":
		return 5
	case strings.HasPrefix(path, "/messages/"):
		format := strings.ToLower(q.Get("format"))
		if format == "" || format == "raw" || format == "full" || format == "html" {
			return 60
		}
		return 20
	default:
		return 100
	}
}

func mailboxQuotaKey(owner string) string {
	digest := sha256.Sum256([]byte(owner))
	return fmt.Sprintf("gmail/mailbox/%x", digest)
}

func (a *httpAPI) noteQuotaLimit(ctx context.Context, cause error) error {
	limited, ok := errors.AsType[*connector.RateLimitedError](cause)
	if !ok {
		return cause
	}
	delay := limited.RetryAfter
	if delay <= 0 {
		delay = rateBackoffBase
	}
	key := "gmail/project"
	if limited.MailboxScoped() {
		owner, ok := ctx.Value(quotaOwnerKey{}).(string)
		if !ok || owner == "" {
			return cause
		}
		key = mailboxQuotaKey(owner)
	}
	if err := a.quota.meter.Pause(ctx, key, delay); err != nil {
		return errors.Join(cause, fmt.Errorf("gmail: saving shared quota delay: %w", ErrUnreachable))
	}
	return cause
}
