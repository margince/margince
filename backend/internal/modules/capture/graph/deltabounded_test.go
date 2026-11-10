// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// A delta round read page by page and checkpointed. These tests pin what it
// keeps in memory and what it reads again after an interruption. Every way a
// round can break off still ends with the same stored mail and watermark.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const pagedPageSize = 100

// pagedEntry is one entry of the synthetic inbox's delta round.
type pagedEntry struct {
	id      string
	removed bool
}

// pagedProvider serves one inbox delta round in pages of pagedPageSize, and
// the folder's tally. A page listed in failOnce answers 500 the first time it
// is read, one in goneOnce answers 410.
type pagedProvider struct {
	mu        sync.Mutex
	entries   []pagedEntry
	failOnce  map[int]bool
	goneOnce  map[int]bool
	extraHeld int
	pageReads int
	srv       *httptest.Server
}

func newPagedProvider(t *testing.T, entries []pagedEntry) *pagedProvider {
	t.Helper()
	p := &pagedProvider{entries: entries, failOnce: map[int]bool{}, goneOnce: map[int]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", p.servePage)
	mux.HandleFunc("/me/mailFolders/inbox/messages", p.serveTally)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *pagedProvider) servePage(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	page := 0
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, _ = strconv.Atoi(raw)
	}
	p.pageReads++
	if p.failOnce[page] {
		delete(p.failOnce, page)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if p.goneOnce[page] {
		delete(p.goneOnce, page)
		w.WriteHeader(http.StatusGone)
		return
	}
	from := page * pagedPageSize
	to := min(from+pagedPageSize, len(p.entries))
	value := make([]map[string]any, 0, to-from)
	for _, e := range p.entries[from:to] {
		if e.removed {
			value = append(value, map[string]any{"id": e.id, "@removed": map[string]string{"reason": "deleted"}})
			continue
		}
		value = append(value, map[string]any{"id": e.id, "receivedDateTime": anchorAfter.Add(time.Hour).Format(time.RFC3339)})
	}
	body := map[string]any{"value": value}
	if to < len(p.entries) {
		body["@odata.nextLink"] = p.srv.URL + "/me/mailFolders/inbox/messages/delta?page=" + strconv.Itoa(page+1)
	} else {
		body["@odata.deltaLink"] = p.srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=final"
	}
	writeJSON(w, body)
}

// serveTally answers what the folder holds now: each message whose last entry
// in the round is not a tombstone, plus extraHeld the round never names.
func (p *pagedProvider) serveTally(w http.ResponseWriter, _ *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	last := map[string]bool{}
	for _, e := range p.entries {
		last[e.id] = e.removed
	}
	held := p.extraHeld
	for _, removed := range last {
		if !removed {
			held++
		}
	}
	writeJSON(w, map[string]any{"value": []map[string]any{}, "@odata.count": held})
}

func (p *pagedProvider) reads() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pageReads
}

// pagedAPI walks the provider over HTTP and serves message bodies from memory.
// A round of 100k entries then costs its paging, not 100k HTTP fetches.
type pagedAPI struct {
	API
	fetches int
}

func (a *pagedAPI) GetMIME(_ context.Context, _, _, id string) ([]byte, error) {
	a.fetches++
	return rawMsg(id+"@test.example", "sender@test.example"), nil
}

// savedCursors keeps every checkpoint a sync saved, as the registry would.
type savedCursors struct{ cursors []connector.Cursor }

func (s *savedCursors) CheckpointSync(_ context.Context, cursor connector.Cursor) error {
	s.cursors = append(s.cursors, cursor)
	return nil
}

func (s *savedCursors) latest(t *testing.T) cursorState {
	t.Helper()
	if len(s.cursors) == 0 {
		return cursorState{}
	}
	state, err := parseCursor(s.cursors[len(s.cursors)-1])
	if err != nil {
		t.Fatalf("reading the last checkpoint: %v", err)
	}
	return state
}

// pagedRun is what one round stored: the distinct messages captured and the
// watermark it closed on.
type pagedRun struct {
	captured []string
	link     string
	restarts int
}

// runRound pulls the inbox until it closes, resuming from the last checkpoint
// after every failure, as the next sync cycle would.
func runRound(t *testing.T, p *pagedProvider, start cursorState) (pagedRun, *pagedAPI) {
	t.Helper()
	api := &pagedAPI{API: NewAPI(p.srv.Client(), p.srv.URL)}
	conn := New(&fakeOAuth{access: "access-1"}, api)
	sink := &recordingSink{}
	saved := &savedCursors{}
	ctx := connector.WithSyncCheckpointer(context.Background(), saved)
	state := start
	run := pagedRun{}
	for {
		link, err := conn.pullFolder(ctx, "access", folderInbox, &state, anchorAfter, sink, owner)
		if err == nil {
			run.link = strings.TrimPrefix(link, p.srv.URL)
			break
		}
		run.restarts++
		// A planted failure fires once, so a round restarts at most once a page.
		if run.restarts > pagesOf(p.entries)+1 {
			t.Fatalf("the round never closed: %v", err)
		}
		state = saved.latest(t)
		if len(state.Rounds) == 0 {
			state = start
		}
	}
	for i, cursor := range saved.cursors {
		mid, err := parseCursor(cursor)
		if err != nil || mid.DeltaLink != start.DeltaLink || mid.Rounds[folderInbox] == nil || mid.Rounds[folderInbox].NextLink == "" {
			t.Fatalf("checkpoint %d is not a round in progress over the prior watermark: %s (%v)", i, cursor, err)
		}
	}
	seen := map[string]bool{}
	for _, rec := range sink.recs {
		seen[rec.ProviderMessageID] = true
	}
	for id := range seen {
		run.captured = append(run.captured, id)
	}
	slices.Sort(run.captured)
	return run, api
}

// pagedEntries is a round of n entries: live messages, then the shapes a real
// round mixes in. Those are a duplicate, a later tombstone, and a move (a
// tombstone for the old id, then the new id).
func pagedEntries(n int) []pagedEntry {
	entries := make([]pagedEntry, 0, n)
	for i := range n - 4 {
		entries = append(entries, pagedEntry{id: "m" + strconv.Itoa(i)})
	}
	return append(entries,
		pagedEntry{id: "m0"},
		pagedEntry{id: "m1", removed: true},
		pagedEntry{id: "m2", removed: true},
		pagedEntry{id: "m2-moved"},
	)
}

func pagesOf(entries []pagedEntry) int {
	return (len(entries) + pagedPageSize - 1) / pagedPageSize
}

func TestADeltaRoundHoldsOnePageAndAnExactCensus(t *testing.T) {
	for _, n := range []int{1_000, 10_000, 100_000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			entries := pagedEntries(n)
			p := newPagedProvider(t, entries)
			var census *AnchorCensus
			largestPage := 0
			link, err := NewAPI(p.srv.Client(), p.srv.URL).DeltaInit(context.Background(), "access", folderInbox, anchorAfter, nil,
				func(_ context.Context, page DeltaPage) error {
					largestPage = max(largestPage, len(page.Messages))
					census = page.Resume.Anchor
					return nil
				})
			if err != nil {
				t.Fatal(err)
			}
			if largestPage > pagedPageSize {
				t.Errorf("a page handed over %d messages, more than one provider page", largestPage)
			}
			// The moved id adds one; the duplicate and tombstones add none.
			if got, want := len(census.seen), n-4+1; got != want {
				t.Errorf("the census holds %d ids, want each distinct message once: %d", got, want)
			}
			if p.reads() != pagesOf(entries) || link == "" {
				t.Errorf("read %d pages for a round of %d and closed on %q", p.reads(), pagesOf(entries), link)
			}
		})
	}
}

// A restart at every page boundary (every 50th at 100k) ends where an
// uninterrupted round ends. Each restart reads again at most one checkpoint's
// worth of pages.
func TestAnInterruptedRoundConvergesOnTheUninterruptedOne(t *testing.T) {
	for _, tc := range []struct{ n, every int }{{1_000, 1}, {10_000, 1}, {100_000, 50}} {
		t.Run(strconv.Itoa(tc.n), func(t *testing.T) {
			entries := pagedEntries(tc.n)
			whole, wholeAPI := runRound(t, newPagedProvider(t, entries), cursorState{})
			if whole.restarts != 0 {
				t.Fatalf("the uninterrupted round restarted %d times", whole.restarts)
			}
			p := newPagedProvider(t, entries)
			pages := pagesOf(entries)
			for page := tc.every; page < pages; page += tc.every {
				p.failOnce[page] = true
			}
			p.failOnce[pages-1] = true
			broken, brokenAPI := runRound(t, p, cursorState{})
			if broken.restarts == 0 {
				t.Fatal("no restart happened")
			}
			if !slices.Equal(broken.captured, whole.captured) || broken.link != whole.link {
				t.Fatalf("after %d restarts: %d messages and %q, want %d and %q",
					broken.restarts, len(broken.captured), broken.link, len(whole.captured), whole.link)
			}
			// Each restart reads again what followed its last checkpoint.
			reread := p.reads() - pages
			if limit := broken.restarts * (checkpointEveryPages + 1); reread > limit {
				t.Errorf("%d restarts read %d pages again, more than %d", broken.restarts, reread, limit)
			}
			if extra, limit := brokenAPI.fetches-wholeAPI.fetches, broken.restarts*checkpointEveryPages*pagedPageSize; extra > limit {
				t.Errorf("%d restarts fetched %d messages again, more than %d", broken.restarts, extra, limit)
			}
		})
	}
}

func TestA410MidRoundReanchorsAndConverges(t *testing.T) {
	entries := pagedEntries(2_000)
	whole, _ := runRound(t, newPagedProvider(t, entries), cursorState{})
	for name, start := range map[string]cursorState{
		"anchor":      {},
		"incremental": {DeltaLink: "PLACEHOLDER"},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPagedProvider(t, entries)
			if start.DeltaLink != "" {
				start.DeltaLink = p.srv.URL + "/me/mailFolders/inbox/messages/delta?page=0"
			}
			p.goneOnce[13] = true
			got, _ := runRound(t, p, start)
			if !slices.Equal(got.captured, whole.captured) || got.link != whole.link {
				t.Fatalf("after a 410: %d messages and %q, want %d and %q", len(got.captured), got.link, len(whole.captured), whole.link)
			}
		})
	}
}

// cancelingAPI cancels the round's context at one message, as a stopped job
// does. The tests stop on a checkpoint's own page, so a checkpoint saved
// before its page is captured would skip the rest of that page.
type cancelingAPI struct {
	*pagedAPI
	at     string
	cancel context.CancelFunc
}

func (a *cancelingAPI) GetMIME(ctx context.Context, token, folder, id string) ([]byte, error) {
	if id == a.at && a.cancel != nil {
		a.cancel()
		a.cancel = nil
		return nil, ctx.Err()
	}
	return a.pagedAPI.GetMIME(ctx, token, folder, id)
}

func TestACancelledRoundResumesWhereItsCheckpointStands(t *testing.T) {
	entries := pagedEntries(3_500)
	whole, _ := runRound(t, newPagedProvider(t, entries), cursorState{})
	p := newPagedProvider(t, entries)
	api := &cancelingAPI{pagedAPI: &pagedAPI{API: NewAPI(p.srv.Client(), p.srv.URL)}, at: "m2950"}
	conn := New(&fakeOAuth{access: "access-1"}, api)
	sink := &recordingSink{}
	saved := &savedCursors{}
	ctx, cancel := context.WithCancel(connector.WithSyncCheckpointer(context.Background(), saved))
	api.cancel = cancel
	state := cursorState{}
	if _, err := conn.pullFolder(ctx, "access", folderInbox, &state, anchorAfter, sink, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled round answered %v", err)
	}
	resumed := saved.latest(t)
	if resumed.Rounds[folderInbox] == nil {
		t.Fatal("a round cancelled on page 30 left no checkpoint")
	}
	link, err := conn.pullFolder(connector.WithSyncCheckpointer(context.Background(), saved), "access", folderInbox, &resumed, anchorAfter, sink, owner)
	if err != nil || strings.TrimPrefix(link, p.srv.URL) != whole.link {
		t.Fatalf("the resumed round closed on %q: %v", link, err)
	}
	captured := map[string]bool{}
	for _, rec := range sink.recs {
		captured[rec.ProviderMessageID] = true
	}
	if len(captured) != len(whole.captured) {
		t.Fatalf("captured %d messages, want %d", len(captured), len(whole.captured))
	}
}

// A round short of the folder's tally is refused, before and after a restart,
// and stores no watermark.
func TestAShortRoundIsRefusedEvenAcrossACheckpoint(t *testing.T) {
	p := newPagedProvider(t, pagedEntries(3_000))
	// Two tombstones lower the tally by two: three more held is one short.
	p.extraHeld = 3
	p.failOnce[20] = true
	api := &pagedAPI{API: NewAPI(p.srv.Client(), p.srv.URL)}
	conn := New(&fakeOAuth{access: "access-1"}, api)
	saved := &savedCursors{}
	ctx := connector.WithSyncCheckpointer(context.Background(), saved)
	state := cursorState{}
	if _, err := conn.pullFolder(ctx, "access", folderInbox, &state, anchorAfter, &recordingSink{}, owner); err == nil {
		t.Fatal("the round with a failed page closed")
	}
	resumed := saved.latest(t)
	if _, err := conn.pullFolder(ctx, "access", folderInbox, &resumed, anchorAfter, &recordingSink{}, owner); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a round one short of the tally answered %v, want it refused", err)
	}
}

func TestAnAnchorPastTheCapStopsLoudly(t *testing.T) {
	p := newPagedProvider(t, pagedEntries(anchorSeenCap+4))
	_, err := NewAPI(p.srv.Client(), p.srv.URL).DeltaInit(context.Background(), "access", folderInbox, anchorAfter, nil,
		func(context.Context, DeltaPage) error { return nil })
	if !errors.Is(err, ErrAnchorTooLarge) {
		t.Fatalf("a round of %d distinct messages answered %v, want ErrAnchorTooLarge", anchorSeenCap+1, err)
	}
	if !strings.Contains(err.Error(), "run the history import for this mailbox, then re-anchor it") {
		t.Errorf("the fault %q does not tell the operator what to do", err)
	}
}

func TestAStoredCensusPastTheCapIsRefused(t *testing.T) {
	seen := make([]string, anchorSeenCap+1)
	for i := range seen {
		seen[i] = fmt.Sprint(i)
	}
	stored, err := json.Marshal(anchorCensusJSON{After: anchorAfter, Opened: pinned, Seen: seen})
	if err != nil {
		t.Fatal(err)
	}
	var census AnchorCensus
	if err := json.Unmarshal(stored, &census); err == nil {
		t.Fatal("a stored census past the cap was resumed")
	}
}

// A first sync checkpointed in Sent Items has no watermark yet. The registry
// then passes a fresh window, and the inbox still anchors where Sent Items did.
func TestAResumedFirstSyncAnchorsTheInboxWhereItBegan(t *testing.T) {
	held := anchorAfter.Add(-24 * time.Hour)
	stored, err := json.Marshal(cursorState{Email: owner, Rounds: map[string]*DeltaResume{
		folderSent: {NextLink: "https://graph.microsoft.com/v1.0/me/mailFolders/sentitems/messages/delta?page=3", Anchor: newAnchorCensus(held, pinned)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{initDelta: "inbox-link", sentInitDelta: "sent-link"}
	if _, err := pinnedConn(api).SyncFrom(context.Background(), authBytes(t), stored, anchorAfter, &recordingSink{}); err != nil {
		t.Fatal(err)
	}
	if !api.initAfter.Equal(held) {
		t.Fatalf("the inbox anchored from %s, want the held %s", api.initAfter, held)
	}
}
