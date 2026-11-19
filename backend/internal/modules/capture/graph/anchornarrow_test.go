// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// spreadPageSize keeps a round of a hundred thousand messages to a hundred
// pages, so walking it twice stays cheap.
const spreadPageSize = 1_000

// spreadProvider is an inbox of n messages one minute apart, ending an hour
// before the test starts. Unlike pagedProvider it honours the receivedDateTime
// filter on both the delta and the tally, which is what narrowing bisects on.
//
// churn ids appear in a round from the oldest message on and are not in the
// tally, as messages moved out while the round ran.
type spreadProvider struct {
	base  time.Time
	n     int
	churn int
	srv   *httptest.Server
}

func newSpreadProvider(t *testing.T, n int) *spreadProvider {
	t.Helper()
	p := &spreadProvider{n: n, base: clocktest.Now(t).Add(-time.Duration(n+60) * time.Minute).Truncate(time.Minute)}
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", p.servePage)
	mux.HandleFunc("/me/mailFolders/inbox/messages", p.serveTally)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// api reads p over HTTP at an hour past its newest message.
func (p *spreadProvider) api() *httpAPI {
	api := newHTTPAPI(p.srv.Client(), p.srv.URL, personalMailbox)
	api.now = func() time.Time { return p.received(p.n).Add(time.Hour) }
	return api
}

func (p *spreadProvider) received(i int) time.Time { return p.base.Add(time.Duration(i) * time.Minute) }

// firstAtOrAfter is the index of the first message received at or after t.
func (p *spreadProvider) firstAtOrAfter(t time.Time) int {
	if !t.After(p.base) {
		return 0
	}
	return min(p.n, int((t.Sub(p.base)+time.Minute-1)/time.Minute))
}

// window reads the filter's bounds: "receivedDateTime ge A" with an optional
// "and receivedDateTime lt B".
func (p *spreadProvider) window(filter string) (from, to int, err error) {
	fields := strings.Fields(filter)
	if len(fields) != 3 && len(fields) != 7 {
		return 0, 0, errors.New("unexpected filter " + filter)
	}
	after, err := time.Parse(time.RFC3339Nano, fields[2])
	if err != nil {
		return 0, 0, err
	}
	from, to = p.firstAtOrAfter(after), p.n
	if len(fields) == 7 {
		before, err := time.Parse(time.RFC3339Nano, fields[6])
		if err != nil {
			return 0, 0, err
		}
		to = p.firstAtOrAfter(before)
	}
	return from, to, nil
}

// pageStart is where a delta request reads from: the filter's lower bound on
// the first page, and the position a nextLink carries after it.
func (p *spreadProvider) pageStart(q url.Values) (first, page int, err error) {
	if q.Get("from") == "" {
		first, _, err = p.window(q.Get(paramFilter))
		return first, 0, err
	}
	if first, err = strconv.Atoi(q.Get("from")); err != nil {
		return 0, 0, err
	}
	page, err = strconv.Atoi(q.Get("page"))
	return first, page, err
}

func (p *spreadProvider) servePage(w http.ResponseWriter, r *http.Request) {
	first, page, err := p.pageStart(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	from := first + page*spreadPageSize
	to := min(from+spreadPageSize, p.n)
	value := make([]map[string]any, 0, to-from)
	if from == 0 {
		for i := range p.churn {
			value = append(value, map[string]any{"id": "churn" + strconv.Itoa(i), "receivedDateTime": p.base.Format(time.RFC3339)})
		}
	}
	for i := from; i < to; i++ {
		value = append(value, map[string]any{"id": "m" + strconv.Itoa(i), "receivedDateTime": p.received(i).Format(time.RFC3339)})
	}
	body := map[string]any{"value": value}
	if to < p.n {
		body["@odata.nextLink"] = p.srv.URL + "/me/mailFolders/inbox/messages/delta?from=" + strconv.Itoa(first) + "&page=" + strconv.Itoa(page+1)
	} else {
		body["@odata.deltaLink"] = p.srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=final"
	}
	writeJSON(w, body)
}

func (p *spreadProvider) serveTally(w http.ResponseWriter, r *http.Request) {
	from, to, err := p.window(r.URL.Query().Get(paramFilter))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"value": []map[string]any{}, "@odata.count": max(0, to-from)})
}

// assertFits checks that a narrowed bound is the earliest one whose tally fits,
// to within the bisection's resolution.
func (p *spreadProvider) assertFits(t *testing.T, fits time.Time) {
	t.Helper()
	earliest := p.received(p.n - anchorFitTarget)
	if fits.Before(earliest) || fits.After(earliest.Add(anchorFitResolution)) {
		t.Errorf("the anchor narrowed to %s, want the earliest point that fits, %s, within %s", fits, earliest, anchorFitResolution)
	}
}

// narrowRecorder is the engine's side of a narrowing: it keeps each span it is
// told and each checkpoint saved after one.
type narrowRecorder struct {
	spans       [][2]time.Time
	afterNarrow []connector.Cursor
}

func (n *narrowRecorder) CheckpointSync(_ context.Context, cursor connector.Cursor) error {
	if len(n.spans) > 0 {
		n.afterNarrow = append(n.afterNarrow, cursor)
	}
	return nil
}

func (n *narrowRecorder) NarrowSyncAnchor(_ context.Context, from, to time.Time) error {
	n.spans = append(n.spans, [2]time.Time{from, to})
	return nil
}

// A fresh window the tally already puts past the cap narrows before the walk,
// so no page is read and no message fetched twice.
func TestAFreshAnchorPastTheCapNarrowsBeforeWalking(t *testing.T) {
	p := newSpreadProvider(t, anchorSeenCap+20_000)
	pages := 0
	_, err := p.api().DeltaInit(context.Background(), "access", folderInbox, p.base, nil,
		func(context.Context, DeltaPage) error { pages++; return nil })
	assertNarrowed(t, p, err)
	if pages != 0 {
		t.Errorf("the round walked %d page(s) before narrowing, want none", pages)
	}
}

// A window that fits by the tally can still outgrow the census while it is
// walked, as mail moves. That round narrows too.
func TestAnAnchorThatOutgrowsTheCapWhileWalkedNarrows(t *testing.T) {
	p := newSpreadProvider(t, anchorSeenCap-1_000)
	p.churn = 2_000
	pages := 0
	_, err := p.api().DeltaInit(context.Background(), "access", folderInbox, p.base, nil,
		func(context.Context, DeltaPage) error { pages++; return nil })
	assertNarrowed(t, p, err)
	if pages == 0 {
		t.Error("the round narrowed before walking, so this case never reached the census cap")
	}
}

func assertNarrowed(t *testing.T, p *spreadProvider, err error) {
	t.Helper()
	tooLarge, ok := errors.AsType[*AnchorTooLargeError](err)
	if !ok || !errors.Is(err, ErrAnchorTooLarge) {
		t.Fatalf("a round past the cap answered %v, want an AnchorTooLargeError", err)
	}
	if !tooLarge.From.Equal(p.base) {
		t.Errorf("the skipped span starts %s, want the anchor's own bound %s", tooLarge.From, p.base)
	}
	p.assertFits(t, tooLarge.Fits)
}

// narrowedFolder is what one pullFolder over p stored after a narrowing.
type narrowedFolder struct {
	recorder *narrowRecorder
	api      *pagedAPI
	fits     time.Time
}

// pullNarrowed pulls p's inbox, which must narrow once and then close, and
// checks every message of the narrowed window was captured.
func pullNarrowed(t *testing.T, p *spreadProvider) narrowedFolder {
	t.Helper()
	api := &pagedAPI{API: p.api()}
	conn := New(&fakeOAuth{access: "access-1"}, api)
	recorder := &narrowRecorder{}
	ctx := connector.WithSyncCheckpointer(context.Background(), recorder)
	sink := &recordingSink{}

	link, err := conn.pullFolder(ctx, "access", folderInbox, &cursorState{}, p.base, sink, owner)
	if err != nil {
		t.Fatalf("the folder did not close after narrowing: %v", err)
	}
	if !strings.Contains(link, "deltatoken=final") {
		t.Errorf("the folder closed on %q, want the narrowed round's deltaLink", link)
	}
	if len(recorder.spans) != 1 || !recorder.spans[0][0].Equal(p.base) {
		t.Fatalf("the engine was told the spans %v, want one starting at %s", recorder.spans, p.base)
	}
	fits := recorder.spans[0][1]
	p.assertFits(t, fits)
	captured := map[string]bool{}
	for _, rec := range sink.recs {
		captured[rec.ProviderMessageID] = true
	}
	for i := p.firstAtOrAfter(fits); i < p.n; i++ {
		if !captured["m"+strconv.Itoa(i)] {
			t.Fatalf("message m%d, received %s inside the narrowed window, was not captured", i, p.received(i))
		}
	}
	return narrowedFolder{recorder: recorder, api: api, fits: fits}
}

// The folder anchors again from the bound that fits, in the same cycle, and
// tells the engine which span it left for the history import. Counted first,
// it fetches only the narrowed window.
func TestAFolderPastTheCapReanchorsNarrowerAndRecordsTheSkippedSpan(t *testing.T) {
	p := newSpreadProvider(t, anchorSeenCap+20_000)
	got := pullNarrowed(t, p)
	if inWindow := p.n - p.firstAtOrAfter(got.fits); got.api.fetches != inWindow {
		t.Errorf("the folder fetched %d messages, want only the %d of the narrowed window", got.api.fetches, inWindow)
	}
}

// A round that trips the cap mid-walk has checkpointed its oversized census. It
// is dropped, so an interrupted retry resumes the narrowed one instead.
func TestAFolderThatTripsTheCapMidWalkDropsTheOversizedRound(t *testing.T) {
	p := newSpreadProvider(t, anchorSeenCap-1_000)
	p.churn = 2_000
	got := pullNarrowed(t, p)
	if len(got.recorder.afterNarrow) == 0 {
		t.Fatal("nothing was saved after the narrowing, so the oversized round is still the stored one")
	}
	if dropped, err := parseCursor(got.recorder.afterNarrow[0]); err != nil || dropped.Rounds[folderInbox] != nil {
		t.Errorf("the first save after the narrowing still holds a round (%v), want the oversized one dropped", err)
	}
	for i, cursor := range got.recorder.afterNarrow {
		state, err := parseCursor(cursor)
		if err != nil {
			t.Fatal(err)
		}
		if round := state.Rounds[folderInbox]; round != nil && !round.Anchor.After.Equal(got.fits) {
			t.Errorf("checkpoint %d after the narrowing resumes a census from %s, want %s", i, round.Anchor.After, got.fits)
		}
	}
}

// With no engine to record the skipped span, the round stops as it did before
// narrowing existed. A span skipped without a record is mail lost silently.
func TestANarrowingNobodyRecordsStopsTheRound(t *testing.T) {
	p := newSpreadProvider(t, anchorSeenCap+20_000)
	api := &pagedAPI{API: p.api()}
	conn := New(&fakeOAuth{access: "access-1"}, api)

	_, err := conn.pullFolder(context.Background(), "access", folderInbox, &cursorState{}, p.base, &recordingSink{}, owner)
	if !errors.Is(err, ErrAnchorTooLarge) || !errors.Is(err, connector.ErrNoAnchorNarrower) {
		t.Fatalf("an unrecorded narrowing answered %v, want the round stopped on both reasons", err)
	}
}
