// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stepClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// loader counts the reads that reach "the database" and answers the row it holds.
type loader struct {
	mu    sync.Mutex
	reads int
	row   json.RawMessage
}

func (l *loader) load() (json.RawMessage, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reads++
	return l.row, l.row != nil, nil
}

func (l *loader) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reads
}

func newTestCache() (*valueCache, *stepClock) {
	clock := &stepClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	return newValueCache(5*time.Second, clock.Now), clock
}

func TestASecondReadInsideTheLifetimeSendsNoQuery(t *testing.T) {
	c, _ := newTestCache()
	l := &loader{row: json.RawMessage(`1`)}
	for range 3 {
		raw, found, err := c.read("a.b", l.load)
		if err != nil || !found || string(raw) != "1" {
			t.Fatalf("read = %s %v %v, want 1 true nil", raw, found, err)
		}
	}
	if got := l.count(); got != 1 {
		t.Fatalf("three reads sent %d queries, want 1", got)
	}
}

func TestAReadAfterTheLifetimeSendsAQueryAgain(t *testing.T) {
	c, clock := newTestCache()
	l := &loader{row: json.RawMessage(`1`)}
	if _, _, err := c.read("a.b", l.load); err != nil {
		t.Fatal(err)
	}
	clock.advance(4 * time.Second)
	if _, _, err := c.read("a.b", l.load); err != nil {
		t.Fatal(err)
	}
	if got := l.count(); got != 1 {
		t.Fatalf("a read at 4s sent %d queries in all, want 1", got)
	}
	clock.advance(time.Second)
	l.row = json.RawMessage(`2`)
	raw, _, err := c.read("a.b", l.load)
	if err != nil || string(raw) != "2" || l.count() != 2 {
		t.Fatalf("a read at 5s = %s (%v) after %d queries, want 2 after 2", raw, err, l.count())
	}
}

func TestAnAbsentRowIsRememberedToo(t *testing.T) {
	c, _ := newTestCache()
	l := &loader{}
	for range 2 {
		if _, found, _ := c.read("a.b", l.load); found {
			t.Fatal("an absent row read as found")
		}
	}
	if got := l.count(); got != 1 {
		t.Fatalf("two reads of an absent row sent %d queries, want 1", got)
	}
}

func TestAWriteIsVisibleAtOnceAndOtherKeysStay(t *testing.T) {
	c, _ := newTestCache()
	l, other := &loader{row: json.RawMessage(`1`)}, &loader{row: json.RawMessage(`9`)}
	_, _, _ = c.read("a.b", l.load)
	_, _, _ = c.read("c.d", other.load)
	l.row = json.RawMessage(`2`)
	c.forget([]string{"a.b"})
	if raw, _, _ := c.read("a.b", l.load); string(raw) != "2" {
		t.Fatalf("after a write the read is %s, want 2", raw)
	}
	_, _, _ = c.read("c.d", other.load)
	if other.count() != 1 {
		t.Fatalf("a write to one key made another key query again (%d)", other.count())
	}
}

// A writer clears the cache before its transaction commits; a read in that gap
// sees the old row and must not keep it.
func TestAReadThatSawTheOldRowAroundAWriteIsNotKept(t *testing.T) {
	c, clock := newTestCache()
	l := &loader{row: json.RawMessage(`1`)}
	_, _, _ = c.read("a.b", func() (json.RawMessage, bool, error) {
		c.forget([]string{"a.b"})
		return json.RawMessage(`1`), true, nil
	})
	l.row = json.RawMessage(`2`)
	if raw, _, _ := c.read("a.b", l.load); string(raw) != "2" {
		t.Fatalf("a read begun before the write kept the old row: got %s", raw)
	}
	if raw, _, _ := c.read("a.b", l.load); string(raw) != "2" || l.count() != 2 {
		t.Fatalf("a read just after a write was kept in the quiet period (%d queries)", l.count())
	}
	clock.advance(5 * time.Second)
	_, _, _ = c.read("a.b", l.load)
	_, _, _ = c.read("a.b", l.load)
	if l.count() != 3 {
		t.Fatalf("after the quiet period reads should be kept again, got %d queries", l.count())
	}
}

func TestACallerCannotChangeWhatTheCacheHolds(t *testing.T) {
	c, _ := newTestCache()
	l := &loader{row: json.RawMessage(`"abc"`)}
	first, _, _ := c.read("a.b", l.load)
	first[1] = 'X'
	second, _, _ := c.read("a.b", l.load)
	if string(second) != `"abc"` {
		t.Fatalf("the cache holds %s after a caller changed its copy", second)
	}
	second[1] = 'Y'
	if third, _, _ := c.read("a.b", l.load); string(third) != `"abc"` {
		t.Fatalf("the cache holds %s after a caller changed a copy it was handed", third)
	}
}

func TestConcurrentReadsAndWritesAreSafe(t *testing.T) {
	c, clock := newTestCache()
	l := &loader{row: json.RawMessage(`1`)}
	var wg sync.WaitGroup
	var wrong atomic.Int32
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				if raw, found, err := c.read("a.b", l.load); err != nil || !found || string(raw) != "1" {
					wrong.Add(1)
				}
				if j%20 == i {
					c.forget([]string{"a.b"})
					clock.advance(time.Second)
				}
			}
		})
	}
	wg.Wait()
	if n := wrong.Load(); n != 0 {
		t.Fatalf("%d concurrent reads answered something other than the stored row", n)
	}
}

// A write transaction open past the lifetime keeps its key out of the cache
// until it ends. A reader then cannot store the row the write has not replaced.
func TestAKeyChangedInAnOpenTransactionIsNotStoredUntilItEnds(t *testing.T) {
	c, clock := newTestCache()
	l := &loader{row: json.RawMessage(`1`)}
	c.hold([]string{"a.b"})
	clock.advance(time.Minute)
	for range 2 {
		if raw, _, _ := c.read("a.b", l.load); string(raw) != "1" {
			t.Fatalf("read %s while the transaction was open, want the committed 1", raw)
		}
	}
	if l.count() != 2 {
		t.Fatalf("a key held by an open transaction was stored (%d queries, want 2)", l.count())
	}
	l.row = json.RawMessage(`2`)
	c.release([]string{"a.b"})
	clock.advance(5 * time.Second)
	if raw, _, _ := c.read("a.b", l.load); string(raw) != "2" {
		t.Fatalf("after the commit the read is %s, want 2", raw)
	}
	_, _, _ = c.read("a.b", l.load)
	if l.count() != 3 {
		t.Fatalf("after the transaction ended reads should be kept again, got %d queries", l.count())
	}
}
