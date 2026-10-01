// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

// The pooled page walk: headers first, a few fetches at once, captures in the
// page's own order, a rate limit waited out inside the page, and the provider's
// wait read from its error message when it sends no header.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// refusingSink records every capture in order and refuses the ids it names.
type refusingSink struct {
	refuse map[string]bool
	recs   []connector.NormalizedRecord
}

func (s *refusingSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	for id := range s.refuse {
		if strings.Contains(rec.NaturalKey.SourceID, strings.TrimSuffix(id, "@mail.gmail.com")) {
			return datasource.EntityRef{}, errSinkRefused
		}
	}
	s.recs = append(s.recs, rec)
	return datasource.EntityRef{}, nil
}

var errSinkRefused = &sinkError{"capture refused the message"}

type sinkError struct{ msg string }

func (e *sinkError) Error() string { return e.msg }

// judgingSink is a recordingSink that also answers the pre-store question,
// dropping every message whose sender is on the dropped domain.
type judgingSink struct {
	recordingSink
	dropDomain string
	judged     []string
}

func (s *judgingSink) DropBeforeStore(_ context.Context, rec connector.NormalizedRecord) (bool, error) {
	s.judged = append(s.judged, rec.NaturalKey.SourceID)
	for _, a := range rec.Addresses {
		if !strings.HasSuffix(a, "@"+s.dropDomain) {
			return false, nil
		}
	}
	return len(rec.Addresses) > 0, nil
}

// headerAPI serves headers as well as full messages, and counts the full
// downloads, so a test can see which messages were never downloaded.
type headerAPI struct {
	pagedAPI
	mu        sync.Mutex
	fullReads map[string]int
	// after holds the full read of an id until the read of another id has
	// finished, so fetches finish out of the page's order.
	after map[string]string
	done  map[string]chan struct{}
	// limitedOnce answers the first full read of each named id with a rate
	// limit carrying a short Retry-After.
	limitedOnce map[string]bool
}

func (h *headerAPI) GetHeaders(_ context.Context, _, id string) (Message, error) {
	raw := h.raws[id]
	head, _, _ := strings.Cut(string(raw), "\r\n\r\n")
	return Message{RFC822: []byte(head + "\r\n\r\n")}, nil
}

func (h *headerAPI) GetRaw(ctx context.Context, access, id string) (Message, error) {
	h.mu.Lock()
	if h.fullReads == nil {
		h.fullReads = map[string]int{}
	}
	h.fullReads[id]++
	first := h.fullReads[id] == 1
	h.mu.Unlock()
	if first && h.limitedOnce[id] {
		return Message{}, &connector.RateLimitedError{RetryAfter: 20 * time.Millisecond}
	}
	if other, ok := h.after[id]; ok {
		select {
		case <-h.done[other]:
		case <-ctx.Done():
			return Message{}, ctx.Err()
		}
	}
	msg, err := h.pagedAPI.GetRaw(ctx, access, id)
	if ch, ok := h.done[id]; ok {
		close(ch)
	}
	return msg, err
}

func pageOf(ids ...string) *headerAPI {
	api := &headerAPI{pagedAPI: pagedAPI{pages: map[string][]string{"": ids}}}
	api.raws = map[string][]byte{}
	return api
}

func TestBackfillPageSkipsColleagueMailWithoutDownloadingIt(t *testing.T) {
	api := pageOf("m1@mail.gmail.com", "m2@mail.gmail.com")
	// m1 is between colleagues; m2 is from a customer.
	api.raws["m1@mail.gmail.com"] = rawMsg("m1@mail.gmail.com", "colleague@myco.com")
	api.raws["m2@mail.gmail.com"] = rawMsg("m2@mail.gmail.com", "alice@acme.com")
	c := New(fakeOAuth{access: "access-1"}, api)
	sink := &judgingSink{dropDomain: "myco.com"}

	res, err := c.BackfillPage(context.Background(), authBytes(t), time.Now(), "", sink)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if api.fullReads["m1@mail.gmail.com"] != 0 {
		t.Error("colleague mail was downloaded in full; its headers already settled it")
	}
	if api.fullReads["m2@mail.gmail.com"] != 1 || len(sink.recs) != 1 {
		t.Errorf("customer mail: %d downloads, %d captures; want one of each", api.fullReads["m2@mail.gmail.com"], len(sink.recs))
	}
	if res.Skipped != 1 || res.Captured != 1 || res.Scanned != 2 {
		t.Errorf("page = %+v, want 1 skipped and 1 captured", res)
	}
}

func TestHeadersOfAMultipartMessageStillParse(t *testing.T) {
	// Most mail is multipart, and the header block carries its Content-Type with
	// no body behind it. If that failed to parse, every such message would be
	// downloaded in full and the headers-first pass would save nothing.
	head := strings.Join([]string{
		"From: colleague@example.com",
		"To: " + owner,
		"Subject: hi",
		"Message-ID: <multi@example.com>",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"", "",
	}, "\r\n")
	sink := &judgingSink{dropDomain: "example.com"}
	drop, err := dropFromHeaders(context.Background(), Message{RFC822: []byte(head)}, sink, owner)
	if err != nil || len(sink.judged) != 1 {
		t.Fatalf("drop=%v err=%v judged=%v; want the header block parsed and judged", drop, err, sink.judged)
	}
}

func TestBackfillPageCapturesInListingOrderWhateverFinishesFirst(t *testing.T) {
	ids := []string{"m1@mail.gmail.com", "m2@mail.gmail.com", "m3@mail.gmail.com", "m4@mail.gmail.com"}
	api := pageOf(ids...)
	// m1 finishes only after m2, and m2 only after m3: all three run at once
	// in a pool of four, so the reads finish in reverse.
	api.after = map[string]string{"m1@mail.gmail.com": "m2@mail.gmail.com", "m2@mail.gmail.com": "m3@mail.gmail.com"}
	api.done = map[string]chan struct{}{"m2@mail.gmail.com": make(chan struct{}), "m3@mail.gmail.com": make(chan struct{})}
	for _, id := range ids {
		api.raws[id] = rawMsg(id, "alice@acme.com")
	}
	c := New(fakeOAuth{access: "access-1"}, api)
	sink := &recordingSink{}
	if _, err := c.BackfillPage(context.Background(), authBytes(t), time.Now(), "", sink); err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(sink.recs) != len(ids) {
		t.Fatalf("captured %d, want %d", len(sink.recs), len(ids))
	}
	for i, rec := range sink.recs {
		if rec.NaturalKey.SourceID != ids[i] {
			t.Fatalf("capture %d was %s, want %s: captures must follow the listing", i, rec.NaturalKey.SourceID, ids[i])
		}
	}
}

func TestBackfillPageWaitsOutAShortRateLimitInsideThePage(t *testing.T) {
	api := pageOf("m1@mail.gmail.com", "m2@mail.gmail.com")
	api.raws["m1@mail.gmail.com"] = rawMsg("m1@mail.gmail.com", "alice@acme.com")
	api.raws["m2@mail.gmail.com"] = rawMsg("m2@mail.gmail.com", "bob@acme.com")
	api.limitedOnce = map[string]bool{"m2@mail.gmail.com": true}
	c := New(fakeOAuth{access: "access-1"}, api)
	res, err := c.BackfillPage(context.Background(), authBytes(t), time.Now(), "", &recordingSink{})
	if err != nil {
		t.Fatalf("a short rate limit ended the page instead of being waited out: %v", err)
	}
	if res.Captured != 2 || api.fullReads["m2@mail.gmail.com"] != 2 {
		t.Fatalf("page = %+v, reads of m2 = %d; want both captured after one retry", res, api.fullReads["m2@mail.gmail.com"])
	}
}

func TestRateLimitedReturnsALongWaitToTheEngine(t *testing.T) {
	calls := 0
	_, err := rateLimited(context.Background(), &rateGate{}, func() (int, error) {
		calls++
		return 0, &connector.RateLimitedError{RetryAfter: time.Hour}
	})
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d; a wait longer than the page may hold must go back to the engine at once", err, calls)
	}
}

func TestRetryAfterIsReadFromGooglesErrorMessage(t *testing.T) {
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"code":429,"message":"User-rate limit exceeded.  Retry after 2026-10-01T05:00:30.000Z"}}`)
	if got := retryAfterInBody(body, now); got != 30*time.Second {
		t.Fatalf("wait = %s, want 30s", got)
	}
	if got := retryAfterInBody([]byte(`{"error":{"message":"Rate Limit Exceeded"}}`), now); got != 0 {
		t.Fatalf("wait = %s, want none when the message names no time", got)
	}
}

func TestHTTPAPIListAfterReportsARejectedPageToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":400,"message":"Invalid pageToken"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	api := NewAPI(srv.Client(), srv.URL)
	if _, _, err := api.ListAfter(context.Background(), "tok", "after:2026/01/01", "stale", 100); err != ErrPageTokenRejected {
		t.Fatalf("err = %v, want ErrPageTokenRejected for a 400 on a page token", err)
	}
	if _, _, err := api.ListAfter(context.Background(), "tok", "after:2026/01/01", "", 100); err == ErrPageTokenRejected {
		t.Fatal("a 400 on the first page names no token and must not be read as a rejected one")
	}
}

func TestHTTPAPIGetHeadersRebuildsTheHeaderBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gone") {
			http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("format") != "metadata" {
			t.Errorf("format = %q, want metadata", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte(`{"labelIds":["SENT","Label_7"],"payload":{"headers":[
			{"name":"From","value":"rep@myco.com"},
			{"name":"To","value":"alice@acme.com,\r\n bob@acme.com"},
			{"name":"Bad:Name","value":"x"}]}}`))
	}))
	defer srv.Close()
	api, ok := NewAPI(srv.Client(), srv.URL).(HeaderFetcher)
	if !ok {
		t.Fatal("the HTTP API must serve headers")
	}

	msg, err := api.GetHeaders(context.Background(), "tok", "m1")
	if err != nil {
		t.Fatalf("GetHeaders: %v", err)
	}
	want := "From: rep@myco.com\r\nTo: alice@acme.com,  bob@acme.com\r\n\r\n"
	if string(msg.RFC822) != want {
		t.Fatalf("header block = %q, want %q: one line per header, folds flattened, a bad name dropped", msg.RFC822, want)
	}
	if !msg.FiledAsSent || len(msg.Labels) != 2 {
		t.Fatalf("labels = %v sent=%v, want both labels and the SENT filing", msg.Labels, msg.FiledAsSent)
	}
	if _, err := api.GetHeaders(context.Background(), "tok", "gone"); err != ErrMessageGone {
		t.Fatalf("a 404 = %v, want ErrMessageGone", err)
	}
}

func TestRateGateKeepsTheLongerPauseAndGivesWayToCancel(t *testing.T) {
	g := &rateGate{}
	g.pause(time.Hour)
	held := g.until
	g.pause(time.Millisecond)
	if !g.until.Equal(held) {
		t.Fatal("a shorter pause shortened a longer one; Gmail's wait must be kept")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.wait(ctx); err == nil {
		t.Fatal("a closed gate must give way to a cancelled context")
	}
	// Held, not passed: a closed gate is still closed when the caller's own
	// deadline ends, which is what the caller sees.
	short, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err := g.wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait on a closed gate = %v, want it held until the caller's deadline", err)
	}
	if err := (&rateGate{}).wait(context.Background()); err != nil {
		t.Fatalf("an open gate held the caller: %v", err)
	}
}
