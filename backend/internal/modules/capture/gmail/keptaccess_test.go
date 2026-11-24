// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// refusingLabels answers 401 to one access token, as Google does for a token
// revoked before its stated lifetime ended.
type refusingLabels struct {
	*fakeAPI
	refused string
	calls   int
}

func (r *refusingLabels) ListLabels(ctx context.Context, access string) ([]connector.NamedContainer, error) {
	r.calls++
	if access == r.refused {
		return nil, &connector.ProviderError{Op: "/labels", Status: http.StatusUnauthorized, Class: ErrAuthRejected}
	}
	return r.fakeAPI.ListLabels(ctx, access)
}

func keepingOAuth(t *testing.T) (OAuth, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintf(w, `{"access_token":"at-%d","expires_in":3600}`, calls.Add(1)); err != nil {
			t.Errorf("writing the token answer: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return NewOAuth(OAuthConfig{
		ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL,
		Tokens: oauthflow.NewTokenCache(func() time.Time { return time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC) }, 8),
	}), &calls
}

func TestASecondCallOnAnUnchangedCredentialAsksForNoToken(t *testing.T) {
	oauth, tokenCalls := keepingOAuth(t)
	c := New(oauth, &fakeAPI{email: owner})
	for range 2 {
		if _, _, err := c.ListContainers(context.Background(), authBytes(t)); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("two calls on one credential made %d token requests, want 1", tokenCalls.Load())
	}
}

func TestARefusedKeptTokenIsRedeemedOnceAndTheCallRepeated(t *testing.T) {
	oauth, tokenCalls := keepingOAuth(t)
	api := &refusingLabels{fakeAPI: &fakeAPI{email: owner}}
	c := New(oauth, api)
	if _, _, err := c.ListContainers(context.Background(), authBytes(t)); err != nil {
		t.Fatal(err)
	}
	api.refused = "at-1"
	if _, _, err := c.ListContainers(context.Background(), authBytes(t)); err != nil {
		t.Fatalf("a revoked kept token failed the call: %v", err)
	}
	if tokenCalls.Load() != 2 || api.calls != 3 {
		t.Fatalf("after a 401 on the kept token: %d token requests and %d listings, want 2 and 3",
			tokenCalls.Load(), api.calls)
	}
}
