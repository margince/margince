// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googleconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The bodies are the shapes Gmail and Calendar answer a throttled call with.
const (
	userRateLimitBody = `{"error":{"code":403,"message":"User-rate limit exceeded.  Retry after 2026-10-01T05:00:00.000Z","errors":[{"message":"User-rate limit exceeded.  Retry after 2026-10-01T05:00:00.000Z","domain":"usageLimits","reason":"userRateLimitExceeded"}],"status":"PERMISSION_DENIED"}}`
	concurrentBody    = `{"error":{"code":429,"message":"Too many concurrent requests for user.","errors":[{"message":"Too many concurrent requests for user.","domain":"global","reason":"rateLimitExceeded"}],"status":"RESOURCE_EXHAUSTED"}}`
	rateLimitBody     = `{"error":{"code":429,"message":"Rate Limit Exceeded","errors":[{"message":"Rate Limit Exceeded","domain":"usageLimits","reason":"rateLimitExceeded"}],"status":"RESOURCE_EXHAUSTED"}}`
	dailyLimitBody    = `{"error":{"code":403,"message":"Daily Limit Exceeded","errors":[{"message":"Daily Limit Exceeded","domain":"usageLimits","reason":"dailyLimitExceeded"}],"status":"PERMISSION_DENIED"}}`
	quotaBody         = `{"error":{"code":403,"message":"Quota exceeded for quota metric 'Queries'.","errors":[{"message":"Quota exceeded","domain":"usageLimits","reason":"quotaExceeded"}],"status":"PERMISSION_DENIED"}}`
	errorInfoBody     = `{"error":{"code":429,"message":"Quota exceeded for quota metric 'Requests' and limit 'Requests per minute per user'.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED","domain":"googleapis.com"}]}}`
	errorInfoQuota    = `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"reason":"QUOTA_EXCEEDED"}]}}`
)

func TestARateLimitIsLabelledWithTheMostSpecificLimitItNames(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"403 per-user limit":      {userRateLimitBody, connector.RateLimitUser},
		"429 concurrency cap":     {concurrentBody, connector.RateLimitConcurrent},
		"429 rate limit":          {rateLimitBody, connector.RateLimitRate},
		"403 daily limit":         {dailyLimitBody, connector.RateLimitDaily},
		"403 project quota":       {quotaBody, connector.RateLimitQuota},
		"ErrorInfo rate limit":    {errorInfoBody, connector.RateLimitRate},
		"ErrorInfo quota":         {errorInfoQuota, connector.RateLimitQuota},
		"generic usage limit":     {`{"error":{"errors":[{"reason":"limitExceeded"}]}}`, connector.RateLimitGeneric},
		"several codes":           {`{"error":{"errors":[{"reason":"rateLimitExceeded"},{"reason":"userRateLimitExceeded"}]}}`, connector.RateLimitUser},
		"a code outside the set":  {`{"error":{"errors":[{"reason":"backendError"}]}}`, connector.RateLimitOther},
		"a status but no code":    {`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`, connector.RateLimitUnspecified},
		"an empty body":           {``, connector.RateLimitUnspecified},
		"a body that is not JSON": {`<html>Too many concurrent requests</html>`, connector.RateLimitUnspecified},
	} {
		if got := RateLimitReason([]byte(tc.body)); got != tc.want {
			t.Errorf("%s: RateLimitReason = %q, want %q", name, got, tc.want)
		}
	}
}

// The shared Google transport, which Calendar reads through, carries the reason
// and status on the rate limit it returns.
func TestGetCarriesTheLimitAndStatusOnARateLimit(t *testing.T) {
	mux := http.NewServeMux()
	for path, tc := range map[string]struct {
		status int
		body   string
	}{
		"/user":       {http.StatusForbidden, userRateLimitBody},
		"/concurrent": {http.StatusTooManyRequests, concurrentBody},
	} {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			//craft:ignore swallowed-errors test stub write
			_, _ = w.Write([]byte(tc.body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for path, want := range map[string]connector.RateLimitedError{
		"/user":       {Reason: connector.RateLimitUser, Status: http.StatusForbidden},
		"/concurrent": {Reason: connector.RateLimitConcurrent, Status: http.StatusTooManyRequests},
	} {
		var out struct{}
		_, err := Get(context.Background(), srv.Client(), srv.URL, "tok", path, nil, &out)
		limited, ok := errors.AsType[*connector.RateLimitedError](err)
		if !ok {
			t.Fatalf("%s: err = %v, want a rate limit", path, err)
		}
		if limited.Reason != want.Reason || limited.Status != want.Status {
			t.Errorf("%s: reason %q status %d, want %q %d", path, limited.Reason, limited.Status, want.Reason, want.Status)
		}
	}
}
