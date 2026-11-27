// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

func TestAllDealRoomLinkGuardsHideTheAddressesRequestHistory(t *testing.T) {
	var firstBody string
	for _, guard := range []string{"caller", "shared", "address"} {
		t.Run(guard, func(t *testing.T) {
			limits := newPublicDealRoomLimiters()
			req := httptest.NewRequest(http.MethodPost, publicDealRoomPrefix+"link-request", strings.NewReader(`{"email":"private@visitor.example"}`))
			req.Header.Set("Content-Type", "application/json")
			bucket, key := limits.linkPerIP, httpserver.ClientIP(req)
			switch guard {
			case "shared":
				bucket, key = limits.linkShared, linkSharedKey
			case "address":
				bucket, key = limits.linkPerEmail, "private@visitor.example"
			}
			for range bucket.Limit() {
				bucket.Record(key)
			}
			edge := publicDealRoom(nil, limits, "https://crm.example.test")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("a refused link request reached its handler") }))
			rec := httptest.NewRecorder()
			edge.ServeHTTP(rec, req)
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("%s: %d %s", guard, rec.Code, rec.Body.String())
			}
			if rec.Header().Get("RateLimit-Limit") != "" || rec.Header().Get("Retry-After") != "3600" || rec.Header().Get("RateLimit-Reset") != "3600" || rec.Header().Get("RateLimit-Remaining") != "0" {
				t.Fatalf("%s exposed its counter: %v", guard, rec.Header())
			}
			if firstBody == "" {
				firstBody = rec.Body.String()
			} else if rec.Body.String() != firstBody {
				t.Fatalf("%s exposed its guard: %s", guard, rec.Body.String())
			}
		})
	}
}

func TestPrivatePacingHeadersNeverRetainAnEarlierCeiling(t *testing.T) {
	t.Parallel()
	headers := http.Header{}
	ratelimit.SetHeaders(headers, 30, 60)
	ratelimit.SetRefusalHeaders(headers, ratelimit.WaitOnly(3600))
	if headers.Get("RateLimit-Limit") != "" || headers.Get("Retry-After") != "3600" {
		t.Fatal(headers)
	}
}
