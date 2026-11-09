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
	"net"
	"regexp"
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
	addr := listenMemWith(t, mem, imapserver.Options{Caps: caps, DebugWriter: wire})
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
	seen := map[int]int{}
	for pulls := 1; ; pulls++ {
		sink := &recordingSink{}
		next, err := c.Sync(context.Background(), auth, cur, sink)
		if err != nil {
			t.Fatalf("pull %d: %v", pulls, err)
		}
		if len(sink.records) > want {
			t.Fatalf("pull %d captured %d messages, over the page of %d", pulls, len(sink.records), want)
		}
		for _, rec := range sink.records {
			m := messageNumber.FindStringSubmatch(rec.NaturalKey.SourceID)
			if m == nil {
				t.Fatalf("record %q names no fixture message", rec.NaturalKey.SourceID)
			}
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatal(err)
			}
			seen[n]++
		}
		if len(sink.records) == 0 {
			return seen, pulls
		}
		cur = next
	}
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

// fakeRanges holds a fixed set of UIDs and counts the ranges asked about.
type fakeRanges struct {
	uids  []imapv2.UID
	asked int
}

func (f *fakeRanges) inRange(_ context.Context, lo, hi uint32) ([]imapv2.UID, error) {
	f.asked++
	var out []imapv2.UID
	for _, u := range f.uids {
		if uint32(u) >= lo && uint32(u) <= hi {
			out = append(out, u)
		}
	}
	return out, nil
}

func TestAGapOfAMillionEmptyUIDsIsCrossedInBoundedSteps(t *testing.T) {
	src := &fakeRanges{uids: []imapv2.UID{1_000_002, 1_000_003}}
	page, err := pageFrom(context.Background(), src, 2, 1_000_004, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.uids) != 2 || page.uids[0] != 1_000_002 {
		t.Fatalf("listed %v, want the two UIDs past the gap", page.uids)
	}
	if src.asked > maxDoublings+1 {
		t.Fatalf("asked %d ranges to cross the gap, over the bound of %d", src.asked, maxDoublings+1)
	}
}

// An empty range below UIDNEXT can never fill, so the watermark may pass it. An
// empty range that reaches UIDNEXT may still receive mail, so it may not.
func TestOnlyARangeBelowUIDNextIsSkippedAsEmpty(t *testing.T) {
	below, err := pageFrom(context.Background(), &fakeRanges{}, 2, 5_000_000, 50)
	if err != nil {
		t.Fatal(err)
	}
	if below.emptyThrough < 2 || below.emptyThrough >= 5_000_000 {
		t.Fatalf("an empty stretch below UIDNEXT reported empty through %d", below.emptyThrough)
	}
	atTop, err := pageFrom(context.Background(), &fakeRanges{}, 2, 100, 50)
	if err != nil {
		t.Fatal(err)
	}
	if atTop.emptyThrough != 0 {
		t.Fatalf("a range reaching UIDNEXT reported empty through %d, want no skip", atTop.emptyThrough)
	}
}
