// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// retiringSink records the listings handed to it for retirement, and fails
// the capture of one event when told to.
type retiringSink struct {
	recordingSink
	failOn   string
	listings [][]string
	until    time.Time
}

func (s *retiringSink) Upsert(ctx context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	if rec.NaturalKey.SourceID == s.failOn {
		return datasource.EntityRef{}, errors.New("the sink is down")
	}
	return s.recordingSink.Upsert(ctx, rec)
}

func (s *retiringSink) RetireUnlistedMeetings(_ context.Context, _ string, listed []string, _, until time.Time) error {
	s.listings = append(s.listings, listed)
	s.until = until
	return nil
}

func TestOnlyAFullySettledRelistRetiresWhatItLeftOut(t *testing.T) {
	listing := func(t *testing.T) [][]byte {
		return [][]byte{
			eventJSON(t, "evt-1", "confirmed", "Kickoff", "2026-07-16T10:00:00Z", gcalOwner, "client@acme.com"),
			eventJSON(t, "evt-2", "cancelled", "Review", "2026-07-17T10:00:00Z", gcalOwner, "client@acme.com"),
		}
	}
	for _, tc := range []struct {
		name    string
		api     func(t *testing.T) *fakeAPI
		failOn  string
		fails   bool
		stale   bool
		retired [][]string
	}{
		{name: "an expired token re-lists", retired: [][]string{{"evt-1", "evt-2"}}, api: func(t *testing.T) *fakeAPI {
			return &fakeAPI{owner: gcalOwner, deltaErr: ErrSyncTokenGone, initialToken: "fresh", initial: listing(t)}
		}},
		{name: "an incremental pull lists only changes", api: func(t *testing.T) *fakeAPI {
			return &fakeAPI{owner: gcalOwner, deltaToken: "next", delta: listing(t)[:1]}
		}},
		{name: "the listing fails", fails: true, api: func(*testing.T) *fakeAPI {
			return &fakeAPI{owner: gcalOwner, deltaErr: ErrSyncTokenGone, initialErr: errors.New("google is down")}
		}},
		{name: "an event of the listing fails to land", failOn: "evt-1", fails: true, api: func(t *testing.T) *fakeAPI {
			return &fakeAPI{owner: gcalOwner, deltaErr: ErrSyncTokenGone, initialToken: "fresh", initial: listing(t)}
		}},
		{name: "a stale list drains the token and retires against the full list alone", stale: true, retired: [][]string{{"evt-1", "evt-2"}}, api: func(t *testing.T) *fakeAPI {
			drained := eventJSON(t, "evt-9", "confirmed", "Moved", "2026-07-18T10:00:00Z", gcalOwner, "client@acme.com")
			return &fakeAPI{owner: gcalOwner, deltaToken: "next", delta: [][]byte{drained}, initialToken: "fresh", initial: listing(t)}
		}},
		{name: "an event names no id", api: func(t *testing.T) *fakeAPI {
			return &fakeAPI{owner: gcalOwner, deltaErr: ErrSyncTokenGone, initialToken: "fresh", initial: append(listing(t), []byte(`{"status":"confirmed"}`))}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
			c := New(fakeOAuth{access: "access-1"}, tc.api(t))
			c.now = func() time.Time { return now }
			sink := &retiringSink{failOn: tc.failOn}
			listedBefore := now.Add(-time.Hour)
			if tc.stale {
				listedBefore = now.Add(-relistAfter)
			}
			prior, err := json.Marshal(cursorState{SyncToken: "sync-abc", ListedAt: listedBefore})
			if err != nil {
				t.Fatal(err)
			}
			next, err := c.Sync(context.Background(), authBytes(t), prior, sink)
			if (err != nil) != tc.fails {
				t.Fatalf("Sync: %v", err)
			}
			// Only a listing that settled dates the window; any other stays due.
			if want := map[bool]time.Time{true: now, false: listedBefore}[tc.retired != nil]; !tc.fails {
				if state, err := parseCursor(next); err != nil || !state.ListedAt.Equal(want) {
					t.Errorf("the cursor dates the listing %v (%v), want %v", state.ListedAt, err, want)
				}
			}
			if !slices.EqualFunc(sink.listings, tc.retired, slices.Equal) {
				t.Fatalf("listings handed for retirement = %v, want %v", sink.listings, tc.retired)
			}
			if tc.retired != nil && !sink.until.Equal(now.Add(captureForwards)) {
				t.Errorf("retirement reaches to %s, want the capture horizon", sink.until)
			}
		})
	}
}
