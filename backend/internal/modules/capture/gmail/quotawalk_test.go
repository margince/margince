// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type quotaHTTP struct {
	mu                            sync.Mutex
	now                           time.Time
	requests, successes, refusals int
	blockedUntil                  time.Time
	ids                           []string
	lateEvery                     int
	usage                         []quotaUse
}

type quotaUse struct {
	at   time.Time
	cost int
}

func (p *quotaHTTP) clock() time.Time { p.mu.Lock(); defer p.mu.Unlock(); return p.now }
func (p *quotaHTTP) advance(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = p.now.Add(d)
	return nil
}

func (p *quotaHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.usage = append(p.usage, quotaUse{at: p.now, cost: getQuotaCost(strings.TrimPrefix(req.URL.Path, "/gmail/v1/users/me"), req.URL.Query())})
	status := http.StatusOK
	headers := make(http.Header)
	var payload string
	if strings.HasSuffix(req.URL.Path, "/messages") {
		messages := make([]map[string]string, 0, len(p.ids))
		for _, id := range p.ids {
			messages = append(messages, map[string]string{"id": id})
		}
		encoded, err := json.Marshal(map[string][]map[string]string{"messages": messages})
		if err != nil {
			return nil, err
		}
		payload = string(encoded)
	} else {
		var err error
		payload, status, err = p.rawResponse(req.URL.Path)
		if err != nil {
			return nil, err
		}
		if status == http.StatusTooManyRequests {
			headers.Set("Retry-After", "120")
		}
	}

	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(payload)), ContentLength: int64(len(payload)), Request: req}, nil
}

func (p *quotaHTTP) rawResponse(path string) (string, int, error) {
	p.requests++
	if p.now.Before(p.blockedUntil) || p.lateEvery > 0 && p.successes == p.lateEvery {
		if !p.now.Before(p.blockedUntil) {
			p.blockedUntil = p.now.Add(2 * time.Minute)
			p.successes = 0
		}
		p.refusals++
		return `{"error":{"code":429,"errors":[{"reason":"userRateLimitExceeded"}]}}`, http.StatusTooManyRequests, nil
	}
	p.successes++
	id := path[strings.LastIndex(path, "/")+1:]
	raw := base64.URLEncoding.EncodeToString(rawMsg(id+"@mail.gmail.com", "sender@acme.com"))
	encoded, err := json.Marshal(map[string]string{"id": id, "raw": raw})
	return string(encoded), http.StatusOK, err
}

func TestGmailRepeatedLateLimitsFinishWithoutReplayingWholePages(t *testing.T) {
	provider := &quotaHTTP{now: time.Unix(0, 0), lateEvery: 25}
	for i := range 100 {
		provider.ids = append(provider.ids, fmt.Sprintf("m%d", i))
	}
	apiValue := NewAPI(&http.Client{Transport: provider}, "https://gmail.test")
	api, ok := apiValue.(*httpAPI)
	if !ok {
		t.Fatal("production Gmail client was not constructed")
	}
	api.quota.now = provider.clock
	api.quota.meter = ratelimit.NewRegistry().NewPacer(provider.clock)
	api.quota.sleep = provider.advance
	connectorInstance := New(fakeOAuth{access: "rotating-access"}, api)
	sink := &recordingSink{}
	token := ""
	totalScanned, failures := 0, 0
	complete := false
	for range 30 {
		saved := &savedHistoryPage{}
		ctx := connector.WithBackfillProgress(t.Context(), saved)
		res, err := connectorInstance.BackfillPage(ctx, authBytes(t), time.Unix(0, 0), token, sink)
		if err == nil {
			totalScanned += res.Scanned
			complete = true
			break
		}
		if !errors.Is(err, connector.ErrRateLimited) {
			t.Fatal(err)
		}
		failures++
		totalScanned += saved.tally.Scanned
		token = saved.cursor
		if token == "" {
			t.Fatal("rate limit lost the prepared listing")
		}
		if !processRawMemory.TryAcquire(rawProcessBudget) {
			t.Fatal("failed page retained its raw memory budget")
		}
		processRawMemory.Release(rawProcessBudget)
		if err := provider.advance(t.Context(), 2*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if !complete || totalScanned != 100 || failures < 3 {
		t.Fatalf("complete=%v scanned=%d failures=%d", complete, totalScanned, failures)
	}
	unique := make(map[string]bool)
	for _, rec := range sink.recs {
		unique[rec.NaturalKey.SourceID] = true
	}
	if len(unique) != 100 || len(sink.recs) > 100+9*failures {
		t.Fatalf("prefix replay unbounded: unique=%d captures=%d failures=%d", len(unique), len(sink.recs), failures)
	}
	t.Logf("completed 100 messages with %d late-limit retries, %d sink calls and %d raw requests", failures, len(sink.recs), provider.requests)
}

func TestGmailHTTPWalkStaysWithinRollingWeightedAllowance(t *testing.T) {
	provider := &quotaHTTP{now: time.Unix(0, 0)}
	for i := range 100 {
		provider.ids = append(provider.ids, fmt.Sprintf("m%d", i))
	}
	value := NewAPI(&http.Client{Transport: provider}, "https://gmail.test")
	api, ok := value.(*httpAPI)
	if !ok {
		t.Fatal("production HTTP client missing")
	}
	api.quota.now = provider.clock
	api.quota.meter = ratelimit.NewRegistry().NewPacer(provider.clock)
	api.quota.sleep = provider.advance
	sink := &recordingSink{}
	result, err := New(fakeOAuth{access: "access"}, api).BackfillPage(t.Context(), authBytes(t), time.Unix(0, 0), "", sink)
	if err != nil || result.Captured != 100 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	peak := 0
	for end, use := range provider.usage {
		units := 0
		for _, earlier := range provider.usage[:end+1] {
			if use.at.Sub(earlier.at) < time.Minute {
				units += earlier.cost
			}
		}
		peak = max(peak, units)
	}
	if peak > defaultMailboxQuota {
		t.Fatalf("rolling weighted usage %d exceeds %d", peak, defaultMailboxQuota)
	}
	if provider.requests != 100 {
		t.Fatalf("unexpected repeated requests %d", provider.requests)
	}
	t.Logf("100 raw messages, peak rolling usage %d units, simulated elapsed %s", peak, provider.clock().Sub(time.Unix(0, 0)))
}
