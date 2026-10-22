// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestChooseAnchorReadsWhereTheLastAttemptWasGoing(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	window := now.Add(-7 * 24 * time.Hour)
	firstAttempt := now.Add(-3 * 24 * time.Hour)
	lapsed := now.Add(-anchorBinds - time.Hour)

	for _, c := range []struct {
		name   string
		held   *time.Time
		from   time.Time
		lapsed bool
	}{
		{"a first attempt starts at the window", nil, window, false},
		{"a retry reads where the first attempt was going", &firstAttempt, firstAttempt, false},
		{"an anchor exactly at the bound still binds", new(now.Add(-anchorBinds)), now.Add(-anchorBinds), false},
		{"a lapsed anchor gives up its point and names the gap", &lapsed, window, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := chooseAnchor(c.held, window, now)
			if !got.from.Equal(c.from) {
				t.Errorf("reads from %s, want %s", got.from, c.from)
			}
			if got.lapsed != c.lapsed {
				t.Errorf("lapsed is %v, want %v", got.lapsed, c.lapsed)
			}
		})
	}
}

// A lapsed anchor must not become the start of the read. Its mail is older than
// the window, so reading from it would stream months on recovery.
func TestChooseAnchorNeverReadsFromALapsedAnchor(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	window := now.Add(-7 * 24 * time.Hour)
	lapsed := now.Add(-200 * 24 * time.Hour)

	got := chooseAnchor(&lapsed, window, now)
	if got.from.Equal(lapsed) {
		t.Fatalf("reads from the lapsed anchor %s", lapsed)
	}
	if !got.from.Equal(window) {
		t.Errorf("reads from %s, want the window %s", got.from, window)
	}
}

// plainPull answers Connector and nothing more, which is what makes it the
// connector a declared window cannot be honoured by.
type plainPull struct{ synced bool }

func (c *plainPull) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "plain_mailbox"}
}

func (c *plainPull) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return nil, nil
}

func (c *plainPull) Sync(
	context.Context, connector.Auth, connector.Cursor, connector.Sink,
) (connector.Cursor, error) {
	c.synced = true
	return nil, nil
}

func (c *plainPull) Normalize(
	context.Context, connector.RawRecord,
) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (c *plainPull) HealthCheck(context.Context, connector.Auth) error { return nil }

type anchoredPull struct {
	plainPull
	toldFrom time.Time
}

func (c *anchoredPull) SyncFrom(
	_ context.Context, _ connector.Auth, _ connector.Cursor,
	from time.Time, _ connector.Sink,
) (connector.Cursor, error) {
	c.toldFrom = from
	return connector.Cursor(`{"ok":true}`), nil
}

func TestSyncFromTellsAnAnchoredConnectorWhereToStart(t *testing.T) {
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	c := &anchoredPull{}

	if _, err := syncFrom(context.Background(), c, nil, nil, at, nil); err != nil {
		t.Fatalf("the pull failed: %v", err)
	}
	if !c.toldFrom.Equal(at) {
		t.Errorf("the connector was told %s, want %s", c.toldFrom, at)
	}
}

// A connector declaring a bounded first sync that cannot be told where to start
// would hold an anchor nobody reads. Every retry would derive its own again,
// which is the defect wearing the fix, so it refuses.
func TestSyncFromRefusesAWindowItCannotHonour(t *testing.T) {
	c := &plainPull{}

	_, err := syncFrom(
		context.Background(), c, nil, nil,
		time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), nil)
	if err == nil {
		t.Fatal("a connector that cannot be told where to start was allowed to pull")
	}
	if !strings.Contains(err.Error(), "cannot be told where to start") {
		t.Errorf("the refusal reads %q, which does not name the cause", err)
	}
	if c.synced {
		t.Error("the refusal still pulled, which would read from the wrong point")
	}
}

// Without an anchor the pull is incremental, and its cursor alone says where to
// read from.
func TestSyncFromTakesThePlainPullWithoutAnAnchor(t *testing.T) {
	c := &anchoredPull{}

	if _, err := syncFrom(context.Background(), c, nil, nil, time.Time{}, nil); err != nil {
		t.Fatalf("the pull failed: %v", err)
	}
	if !c.toldFrom.IsZero() {
		t.Errorf("an unanchored pull told the connector to start at %s", c.toldFrom)
	}
	if !c.synced {
		t.Error("the plain Sync was never reached")
	}
}

// Only a connector that bounds its first sync by time is told where to start.
// Every other one keeps the plain pull, and no row is read or written for it.
func TestSyncStartTellsAnUnboundedConnectorNothing(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	bounded := connector.Descriptor{Name: "test_mailbox", FirstSyncWindow: 7 * 24 * time.Hour}

	for _, c := range []struct {
		name   string
		d      connector.Descriptor
		cursor connector.Cursor
	}{
		{"a connector that declares no window", connector.Descriptor{Name: "plain_mailbox"}, nil},
		{"a connection that already holds a watermark", bounded, connector.Cursor(`{"delta":"x"}`)},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A zero Registry has no database. Reaching one would panic, which
			// is the assertion: neither case may read or write a row.
			from, err := new(Registry).syncStart(
				context.Background(), ids.UUID{}, 1, c.d, c.cursor, now)
			if err != nil {
				t.Fatalf("deciding the start failed: %v", err)
			}
			if !from.IsZero() {
				t.Errorf("the connector was told to start at %s, want the plain pull", from)
			}
		})
	}
}

// The column pair carries a CHECK that the period moves forward. A sync whose
// held anchor still binds has no gap to report. Writing an empty one would be
// refused by the database.
func TestAPeriodThatDoesNotMoveIsNotRecorded(t *testing.T) {
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		name     string
		from, to time.Time
	}{
		{"a period that ends where it starts", at, at},
		{"a period that runs backwards", at, at.Add(-time.Hour)},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A nil transaction stands in for the database the guard must
			// return before reaching.
			recorded, err := recordUnreadPeriod(
				context.Background(), nil, ids.UUID{}, 1, c.from, c.to)
			if err != nil {
				t.Fatalf("recording refused with an error: %v", err)
			}
			if recorded {
				t.Error("an empty period was reported as recorded")
			}
		})
	}
}
