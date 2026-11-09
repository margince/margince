// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// Draining a backlog through the standing sync, with and without ESEARCH. The
// server's wire log counts every UID it sends. A pull that lists the whole
// backlog behind its page makes that count grow with the square of the backlog.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

var (
	fetchResponse   = regexp.MustCompile(`^\* \d+ FETCH `)
	esearchResponse = regexp.MustCompile(`^\* ESEARCH .*COUNT (\d+)`)
	messageNumber   = regexp.MustCompile(`m(\d+)@acme\.test`)
)

// wireCount counts the UIDs a server sends: one per FETCH response, and the
// COUNT of each ESEARCH answer.
type wireCount struct {
	mu        sync.Mutex
	partial   []byte
	uids      int
	esearches int
}

func (w *wireCount) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial = append(w.partial, p...)
	for {
		end := bytes.IndexByte(w.partial, '\n')
		if end < 0 {
			return len(p), nil
		}
		line := w.partial[:end]
		w.partial = w.partial[end+1:]
		if fetchResponse.Match(line) {
			w.uids++
		} else if m := esearchResponse.FindSubmatch(line); m != nil {
			n, err := strconv.Atoi(string(m[1]))
			if err != nil {
				return 0, err
			}
			w.uids += n
			w.esearches++
		}
	}
}

func (w *wireCount) answeredESearch() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.esearches > 0
}

func (w *wireCount) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.uids
}

// backlogServer serves a mailbox holding one anchored message and n pending
// ones above it, every third pending one expunged. It returns the address, the
// wire counter and the message numbers that remain.
func backlogServer(t *testing.T, n int, esearch bool) (string, *wireCount, map[int]bool) {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(memUser, memPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n+1; i++ {
		appendMessage(t, user, rfc822(i))
	}
	mem.AddUser(user)
	caps := imapv2.CapSet{imapv2.CapIMAP4rev1: {}}
	if esearch {
		caps[imapv2.CapESearch] = struct{}{}
	}
	wire := &wireCount{}
	addr := listenMemWith(t, mem, imapserver.Options{Caps: caps, DebugWriter: wire}, nil)
	remaining := map[int]bool{}
	var gone imapv2.UIDSet
	for i := 2; i <= n+1; i++ {
		if i%3 == 0 {
			gone.AddNum(imapv2.UID(i))
			continue
		}
		remaining[i] = true
	}
	expungeUIDs(t, addr, gone)
	return addr, wire, remaining
}

// expungeUIDs deletes the given UIDs through an ordinary client session.
func expungeUIDs(t *testing.T, addr string, uids imapv2.UIDSet) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	client := imapclient.New(conn, &imapclient.Options{})
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("closing the expunge session: %v", err)
		}
	}()
	if err := client.Login(memUser, memPass).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	store := &imapv2.StoreFlags{Op: imapv2.StoreFlagsAdd, Flags: []imapv2.Flag{imapv2.FlagDeleted}, Silent: true}
	if err := client.Store(uids, store, nil).Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Expunge().Close(); err != nil {
		t.Fatal(err)
	}
}

// anchoredAt is a cursor whose watermark sits on the first message, so every
// later message is pending.
func anchoredAt(t *testing.T, addr string) connector.Cursor {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	client := imapclient.New(conn, &imapclient.Options{})
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("closing the anchor session: %v", err)
		}
	}()
	if err := client.Login(memUser, memPass).Wait(); err != nil {
		t.Fatal(err)
	}
	sel, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	return marshalIMAPCursor(imapCursor{UIDValidity: sel.UIDValidity, LastUID: 1, Email: memUser, Mailbox: "INBOX"})
}

// drain syncs until a pull captures nothing, and returns the message numbers
// captured, how often each was, and the number of pulls.
func drain(t *testing.T, c *Connector, auth connector.Auth, cur connector.Cursor, want int) (map[int]int, int) {
	t.Helper()
	return drainFrom(t, c, auth, &cur, want)
}

// drainFrom syncs until a pull captures nothing. It leaves *cur at the last
// cursor, so a test can pull again from where the drain stopped.
func drainFrom(t *testing.T, c *Connector, auth connector.Auth, cur *connector.Cursor, want int) (map[int]int, int) {
	t.Helper()
	seen := map[int]int{}
	for pulls := 1; ; pulls++ {
		sink := &recordingSink{}
		next, err := c.Sync(context.Background(), auth, *cur, sink)
		if err != nil {
			t.Fatalf("pull %d: %v", pulls, err)
		}
		*cur = next
		if len(sink.records) > want {
			t.Fatalf("pull %d captured %d messages, over the page of %d", pulls, len(sink.records), want)
		}
		for _, rec := range sink.records {
			seen[messageOf(t, rec)]++
		}
		if len(sink.records) == 0 || pulls > 1000 {
			return seen, pulls
		}
	}
}

func messageOf(t *testing.T, rec connector.NormalizedRecord) int {
	t.Helper()
	m := messageNumber.FindStringSubmatch(rec.NaturalKey.SourceID)
	if m == nil {
		t.Fatalf("record %q names no fixture message", rec.NaturalKey.SourceID)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestABacklogDrainsOnceWithServerWorkLinearInItsSize(t *testing.T) {
	const page = 50
	for _, n := range []int{1_000, 10_000} {
		for _, esearch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d pending, esearch %v", n, esearch), func(t *testing.T) {
				addr, wire, remaining := backlogServer(t, n, esearch)
				creds := standingCreds(t)
				creds.MaxMessages = page
				c := NewStanding().withDialer(plainDialer(addr))
				cur := anchoredAt(t, addr)
				before := wire.total()

				seen, pulls := drain(t, c, sealCreds(t, creds), cur, page)
				if wire.answeredESearch() != esearch {
					t.Fatalf("the server answered ESEARCH = %v, want %v: the path under test did not run", wire.answeredESearch(), esearch)
				}
				t.Logf("%d pulls, %d UIDs sent", pulls, wire.total()-before)

				for number := range remaining {
					if seen[number] != 1 {
						t.Errorf("message %d was captured %d times, want once", number, seen[number])
					}
				}
				for number, times := range seen {
					if !remaining[number] {
						t.Errorf("message %d was captured %d times, but it is anchored or expunged", number, times)
					}
				}
				// Each body once, plus a bounded listing per pull.
				if sent, bound := wire.total()-before, 2*len(remaining)+pulls*firstSpanPages*page; sent > bound {
					t.Errorf("the server sent %d UIDs over %d pulls, over the linear bound %d", sent, pulls, bound)
				}
			})
		}
	}
}

// A pull that fails leaves the cursor where it was, and the next pull from that
// cursor reads the same page again.
func TestAFailedPullResumesFromTheSameWatermark(t *testing.T) {
	for _, esearch := range []bool{false, true} {
		t.Run(fmt.Sprintf("esearch %v", esearch), func(t *testing.T) {
			addr, _, _ := backlogServer(t, 30, esearch)
			creds := standingCreds(t)
			creds.MaxMessages = 10
			auth := sealCreds(t, creds)
			c := NewStanding().withDialer(plainDialer(addr))
			cur := anchoredAt(t, addr)

			failing := &failAfterSink{after: 3}
			if next, err := c.Sync(context.Background(), auth, cur, failing); err == nil || next != nil {
				t.Fatalf("a pull whose write failed returned cursor %s, error %v; want no cursor and the error", next, err)
			}
			retry := &recordingSink{}
			if _, err := c.Sync(context.Background(), auth, cur, retry); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if len(retry.records) != 10 || retry.records[0].NaturalKey != failing.first {
				t.Fatalf("the retry captured %d messages starting at %q, want the same page of 10 from %q",
					len(retry.records), retry.records[0].NaturalKey.SourceID, failing.first.SourceID)
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if next, err := c.Sync(ctx, auth, cur, &recordingSink{}); !errors.Is(err, context.Canceled) || next != nil {
				t.Fatalf("a cancelled pull returned cursor %s, error %v; want no cursor and context.Canceled", next, err)
			}
		})
	}
}

// failAfterSink records the first key it saw and fails the write after n.
type failAfterSink struct {
	after int
	n     int
	first connector.NaturalKey
}

func (f *failAfterSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	if f.n == 0 {
		f.first = rec.NaturalKey
	}
	f.n++
	if f.n > f.after {
		return datasource.EntityRef{}, errors.New("the store refused the write")
	}
	return datasource.EntityRef{}, nil
}

// A new UIDVALIDITY makes every UID in the cursor meaningless, so the pull
// re-anchors with the bounded window.
func TestANewUIDValidityReanchors(t *testing.T) {
	addr, _, _ := backlogServer(t, 30, false)
	creds := standingCreds(t)
	c := NewStanding().withDialer(plainDialer(addr))
	sink := &recordingSink{}
	cur, err := c.Sync(context.Background(), sealCreds(t, creds), marshalIMAPCursor(imapCursor{
		UIDValidity: 99, LastUID: 2, Email: memUser, Mailbox: "INBOX",
	}), sink)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseIMAPCursor(cur)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.records) != creds.MaxMessages || parsed.UIDValidity == 99 {
		t.Fatalf("captured %d with UIDVALIDITY %d, want the anchor window of %d under the server's generation",
			len(sink.records), parsed.UIDValidity, creds.MaxMessages)
	}
}

// fakeRanges holds a fixed set of UIDs, counts the ranges asked about and
// records the widest one and the largest answer.
type fakeRanges struct {
	uids    []imapv2.UID
	asked   int
	widest  uint32
	largest int
}

func (f *fakeRanges) inRange(_ context.Context, lo, hi uint32, limit int) ([]imapv2.UID, error) {
	f.asked++
	f.widest = max(f.widest, hi-lo+1)
	var out []imapv2.UID
	for _, u := range f.uids {
		if uint32(u) >= lo && uint32(u) <= hi && len(out) < limit {
			out = append(out, u)
		}
	}
	f.largest = max(f.largest, len(out))
	return out, nil
}

func TestAGapOfAMillionEmptyUIDsIsCrossedInBoundedSteps(t *testing.T) {
	src := &fakeRanges{uids: []imapv2.UID{1_000_002, 1_000_003}}
	var page uidPage
	from, pulls := uint32(2), 0
	for ; len(page.uids) == 0 && pulls < 100; pulls++ {
		var err error
		page, err = pageFrom(context.Background(), src, from, 1_000_003, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.uids) == 0 {
			if page.emptyThrough < from {
				t.Fatalf("pull %d found nothing and proved nothing empty, so the next pull repeats it", pulls+1)
			}
			from = page.emptyThrough + 1
		}
	}
	if len(page.uids) != 2 || page.uids[0] != 1_000_002 {
		t.Fatalf("listed %v after %d pulls, want the two UIDs past the gap", page.uids, pulls)
	}
	// Each pull reads at most maxDoublings+1 capped ranges, so a million
	// empty UIDs take a few dozen pulls, never one pull per UID range.
	if pulls > 30 || src.asked > pulls*(maxDoublings+1) {
		t.Fatalf("crossed the gap in %d pulls and %d ranges", pulls, src.asked)
	}
}

// Empty gaps, then a block of thousands of UIDs: no range is wider than the cap
// and no answer carries more than the page.
func TestARangeNeverCarriesMoreThanTheCap(t *testing.T) {
	src := &fakeRanges{uids: []imapv2.UID{1}}
	for u := imapv2.UID(500_000); u < 520_000; u++ {
		src.uids = append(src.uids, u)
	}
	from := uint32(2)
	var page uidPage
	for range 100 {
		var err error
		page, err = pageFrom(context.Background(), src, from, 519_999, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.uids) > 0 {
			break
		}
		from = page.emptyThrough + 1
	}
	if len(page.uids) != 50 || page.uids[0] != 500_000 {
		t.Fatalf("listed %d UIDs from %v, want the first 50 of the dense block", len(page.uids), page.uids[:min(1, len(page.uids))])
	}
	if src.widest > 50*maxSpanPages || src.largest > 50 {
		t.Fatalf("widest range %d, largest answer %d; want at most %d and 50", src.widest, src.largest, 50*maxSpanPages)
	}
}

// A compact ESEARCH answer is read range by range up to the page, never
// expanded: this set would be four billion UIDs.
func TestACompactSetIsReadOnlyUpToThePage(t *testing.T) {
	var set imapv2.UIDSet
	set.AddRange(9, 7)
	set.AddRange(1, 4_000_000_000)
	got := firstInSet(set, 3, math.MaxUint32, 5)
	if want := []imapv2.UID{3, 4, 5, 6, 7}; !slices.Equal(got, want) {
		t.Fatalf("read %v, want %v", got, want)
	}
}

// A range below the highest existing UID that holds nothing never will. A
// server claiming the highest UID itself is empty is not trusted past it.
func TestOnlyARangeBelowTheNewestUIDIsSkippedAsEmpty(t *testing.T) {
	below, err := pageFrom(context.Background(), &fakeRanges{uids: []imapv2.UID{5_000_000}}, 2, 5_000_000, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(below.uids) != 0 || below.emptyThrough < 2 || below.emptyThrough >= 5_000_000 {
		t.Fatalf("an empty stretch below the newest UID listed %v, empty through %d", below.uids, below.emptyThrough)
	}
	lying, err := pageFrom(context.Background(), &fakeRanges{}, 2, 100, 50)
	if err != nil {
		t.Fatal(err)
	}
	if lying.emptyThrough != 0 {
		t.Fatalf("a range up to a newest UID the server did not list reported empty through %d, want no skip", lying.emptyThrough)
	}
}

// steeredSession reports a chosen UIDNEXT, and can hold a FETCH until released.
type steeredSession struct {
	imapserver.Session
	uidNext func(imapv2.UID) imapv2.UID
	stall   func()
}

func (s *steeredSession) Select(mailbox string, options *imapv2.SelectOptions) (*imapv2.SelectData, error) {
	data, err := s.Session.Select(mailbox, options)
	if err == nil && s.uidNext != nil {
		data.UIDNext = s.uidNext(data.UIDNext)
	}
	return data, err
}

func (s *steeredSession) Fetch(w *imapserver.FetchWriter, numSet imapv2.NumSet, options *imapv2.FetchOptions) error {
	if s.stall != nil {
		s.stall()
	}
	return s.Session.Fetch(w, numSet, options)
}

// gapServer holds an anchored message, a run of expunged UIDs wider than the
// first range, and mail above it, served without ESEARCH.
func gapServer(t *testing.T, steer func(imapserver.Session) imapserver.Session) (string, *imapmemserver.User, map[int]bool) {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(memUser, memPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 400; i++ {
		appendMessage(t, user, rfc822(i))
	}
	mem.AddUser(user)
	addr := listenMemWith(t, mem, imapserver.Options{}, steer)
	var gone imapv2.UIDSet
	gone.AddRange(2, 300)
	expungeUIDs(t, addr, gone)
	remaining := map[int]bool{}
	for i := 301; i <= 400; i++ {
		remaining[i] = true
	}
	return addr, user, remaining
}

// The listing reads the mailbox itself, not UIDNEXT. With UIDNEXT missing, too
// low or too high, the mail above an expunged gap is captured once, and so is
// later mail.
func TestAWrongUIDNextNeitherStallsNorSkips(t *testing.T) {
	for name, uidNext := range map[string]func(imapv2.UID) imapv2.UID{
		"missing":  func(imapv2.UID) imapv2.UID { return 0 },
		"too low":  func(imapv2.UID) imapv2.UID { return 5 },
		"too high": func(imapv2.UID) imapv2.UID { return 1_000_000 },
	} {
		t.Run(name, func(t *testing.T) {
			steer := func(s imapserver.Session) imapserver.Session { return &steeredSession{Session: s, uidNext: uidNext} }
			addr, user, remaining := gapServer(t, steer)
			creds := standingCreds(t)
			creds.MaxMessages = 50
			auth := sealCreds(t, creds)
			c := NewStanding().withDialer(plainDialer(addr))
			cur := anchoredAt(t, addr)

			seen, _ := drainFrom(t, c, auth, &cur, 50)
			for number := range remaining {
				if seen[number] != 1 {
					t.Errorf("message %d was captured %d times, want once", number, seen[number])
				}
			}
			appendMessage(t, user, rfc822(401))
			later, _ := drainFrom(t, c, auth, &cur, 50)
			if later[401] != 1 || len(later) != 1 {
				t.Fatalf("after the drain, a new arrival was captured as %v, want message 401 once", later)
			}
		})
	}
}

// An empty mailbox, and one whose newest UID is below the watermark, hold
// nothing new and leave the watermark where it was.
func TestNothingNewLeavesTheWatermark(t *testing.T) {
	for name, messages := range map[string]int{"empty mailbox": 0, "all below the watermark": 3} {
		t.Run(name, func(t *testing.T) {
			addr, _ := startMemServer(t, messages)
			c := NewStanding().withDialer(plainDialer(addr))
			cur := anchoredAt(t, addr)
			parsed, err := parseIMAPCursor(cur)
			if err != nil {
				t.Fatal(err)
			}
			parsed.LastUID = 5
			sink := &recordingSink{}
			next, err := c.Sync(context.Background(), standingAuth(t), marshalIMAPCursor(parsed), sink)
			if err != nil {
				t.Fatal(err)
			}
			after, err := parseIMAPCursor(next)
			if err != nil {
				t.Fatal(err)
			}
			if len(sink.records) != 0 || after.LastUID != 5 {
				t.Fatalf("captured %d and moved the watermark to %d, want nothing and 5", len(sink.records), after.LastUID)
			}
		})
	}
}

// A pull cancelled while the server is still answering returns at once with
// the cancellation and no cursor.
func TestACancelledPullStopsInsideASlowCommand(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	stall := func() {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}
	addr, _, _ := gapServer(t, func(s imapserver.Session) imapserver.Session { return &steeredSession{Session: s, stall: stall} })
	c := NewStanding().withDialer(plainDialer(addr))
	cur := anchoredAt(t, addr)

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		cur connector.Cursor
		err error
	}
	done := make(chan result, 1)
	go func() {
		next, err := c.Sync(ctx, standingAuth(t), cur, &recordingSink{})
		done <- result{next, err}
	}()
	<-entered
	cancel()
	got := <-done
	if !errors.Is(got.err, context.Canceled) || got.cur != nil {
		t.Fatalf("a pull cancelled inside a command returned cursor %s, error %v; want no cursor and context.Canceled", got.cur, got.err)
	}
}
