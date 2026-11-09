// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// overlappingAPI holds the first message's download until the second one has
// started, so a page that fetched one message at a time would never finish.
type overlappingAPI struct {
	*fakeAPI
	secondStarted chan struct{}
}

func (o *overlappingAPI) GetMIME(ctx context.Context, access, folder, id string) ([]byte, error) {
	switch id {
	case "m1":
		select {
		case <-o.secondStarted:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case "m2":
		close(o.secondStarted)
	}
	return o.fakeAPI.GetMIME(ctx, access, folder, id)
}

func TestABackfillPageFetchesAheadAndCapturesInListingOrder(t *testing.T) {
	ids := []string{"m1", "m2", "m3"}
	api := &fakeAPI{listIDs: ids, raws: map[string][]byte{}}
	for _, id := range ids {
		api.raws[id] = rawMsg(id+"@mail.example", "alice@acme.example")
	}
	c := New(&fakeOAuth{access: "access-1"}, &overlappingAPI{fakeAPI: api, secondStarted: make(chan struct{})})
	c.now = func() time.Time { return pinned }
	sink := &recordingSink{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	res, err := c.BackfillPage(ctx, authBytes(t), time.Time{}, "", sink)
	if err != nil {
		t.Fatalf("BackfillPage: %v (a page that fetches one message at a time waits here forever)", err)
	}
	if res.Captured != len(ids) || len(sink.recs) != len(ids) {
		t.Fatalf("tally = %+v with %d records, want all %d captured", res, len(sink.recs), len(ids))
	}
	for i, rec := range sink.recs {
		if want := ids[i] + "@mail.example"; rec.NaturalKey.SourceID != want {
			t.Errorf("capture %d is %s, want %s: captures follow the listing", i, rec.NaturalKey.SourceID, want)
		}
	}
}

// throttledAPI rate-limits the second message while the first is still
// downloading, the way Microsoft answers a mailbox asked too much at once.
type throttledAPI struct {
	*fakeAPI
	limited chan struct{}
}

func (o *throttledAPI) GetMIME(ctx context.Context, access, folder, id string) ([]byte, error) {
	switch id {
	case "m1":
		select {
		case <-o.limited:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case "m2":
		defer close(o.limited)
		return nil, &connector.RateLimitedError{RetryAfter: time.Minute}
	}
	return o.fakeAPI.GetMIME(ctx, access, folder, id)
}

func TestARateLimitEndsTheGraphPageWithoutFetchingTheRest(t *testing.T) {
	ids := []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m10"}
	api := &fakeAPI{listIDs: ids, raws: map[string][]byte{}}
	for _, id := range ids {
		api.raws[id] = rawMsg(id+"@mail.example", "alice@acme.example")
	}
	c := New(&fakeOAuth{access: "access-1"}, &throttledAPI{fakeAPI: api, limited: make(chan struct{})})
	sink := &recordingSink{}

	_, err := c.BackfillPage(t.Context(), authBytes(t), time.Time{}, "", sink)
	if !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("err = %v, want the rate limit, so the engine waits it out", err)
	}
	if len(sink.recs) != 1 {
		t.Errorf("%d captured, want only m1, the message before the limit", len(sink.recs))
	}
}

// Every fetch error but a refused message ends a Graph page, so each stops the
// look-ahead. A refused message is skipped and the walk goes on.
func TestEveryPageEndingFetchErrorHaltsTheGraphWalk(t *testing.T) {
	for name, err := range map[string]error{
		"rate limit":  &connector.RateLimitedError{RetryAfter: time.Minute},
		"unreachable": ErrUnreachable,
	} {
		if !backfillWalk.Halts(err) {
			t.Errorf("%s does not halt the walk; the rest of the look-ahead would still be sent", name)
		}
	}
	if backfillWalk.Halts(fmt.Errorf("message gone: %w", connector.ErrSkip)) {
		t.Error("a refused message halts the walk; it is skipped and the page goes on")
	}
}
