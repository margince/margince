// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

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

// refusingFolders answers 401 to one access token, as Microsoft does for a
// token revoked before its stated lifetime ended.
type refusingFolders struct {
	*fakeAPI
	refused string
	calls   int
}

func (r *refusingFolders) ListFolders(ctx context.Context, access string) ([]connector.NamedContainer, bool, error) {
	r.calls++
	if access == r.refused {
		return nil, false, &connector.ProviderError{Op: "/me/mailFolders", Status: http.StatusUnauthorized, Class: ErrAuthRejected}
	}
	return r.fakeAPI.ListFolders(ctx, access)
}

// rotatingOAuth is Microsoft's token endpoint: every redemption issues a new
// refresh token beside a one-hour access token.
func rotatingOAuth(t *testing.T) (OAuth, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		fmt.Fprintf(w, `{"access_token":"at-%d","refresh_token":"r-%d","expires_in":3600}`, n, n+1)
	}))
	t.Cleanup(srv.Close)
	return NewOAuth(OAuthConfig{
		ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL,
		Tokens: oauthflow.NewTokenCache(func() time.Time { return time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC) }, 8),
	}), &calls
}

func TestASecondSyncOnTheStoredRotationAsksForNoTokenAndRotatesNothing(t *testing.T) {
	oauth, tokenCalls := rotatingOAuth(t)
	rotations := &recordingRotations{}
	c := New(oauth, &fakeAPI{email: owner, initDelta: "https://graph/delta?$1"}).WithCredentialSink(rotations)
	if _, err := c.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
		t.Fatal(err)
	}
	if len(rotations.got) != 1 {
		t.Fatalf("the first sync reported %d rotations, want 1", len(rotations.got))
	}
	if _, err := c.Sync(context.Background(), rotations.got[0], nil, &recordingSink{}); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 1 || rotations.seen != 1 {
		t.Fatalf("second sync on the stored rotation: %d token requests and %d rotations in all, want 1 and 1",
			tokenCalls.Load(), rotations.seen)
	}
}

func TestARefusedKeptTokenIsRedeemedOnceAndTheCallRepeated(t *testing.T) {
	oauth, tokenCalls := rotatingOAuth(t)
	rotations := &recordingRotations{}
	api := &refusingFolders{fakeAPI: &fakeAPI{email: owner, initDelta: "https://graph/delta?$1"}}
	c := New(oauth, api)
	if _, err := c.WithCredentialSink(rotations).Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
		t.Fatal(err)
	}
	api.refused, api.calls = "at-1", 0
	if _, _, err := c.ListContainers(context.Background(), rotations.got[0]); err != nil {
		t.Fatalf("a revoked kept token failed the call: %v", err)
	}
	if tokenCalls.Load() != 2 || api.calls != 2 {
		t.Fatalf("after a 401 on the kept token: %d token requests and %d listings, want 2 and 2",
			tokenCalls.Load(), api.calls)
	}
}
