// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// mimeLatency is what one throttled MIME fetch is taken to cost. The fake
// answers at once and charges this to a virtual clock instead.
const mimeLatency = 6 * time.Second

// throttledMailbox is a shared mailbox's inbox delta. A page holds what the
// client's odata.maxpagesize asks for, and graphPage when it asks for none.
// Its MIME answers advance a virtual clock, and one fetch answers 429.
type throttledMailbox struct {
	messages, throttleAt int

	fetches     int
	elapsed     time.Duration
	preferences []string
}

// graphPage is a large provider page, which would hold more messages than one
// checkpoint span.
const graphPage = 100

func (m *throttledMailbox) serve(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			writeJSON(w, map[string]any{"access_token": "app-tok", "expires_in": 3599})
		case strings.HasSuffix(r.URL.Path, "/$value"):
			m.fetches++
			if m.fetches == m.throttleAt {
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			m.elapsed += mimeLatency
			id := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/messages/")+len("/messages/"):], "/$value")
			_, err := w.Write(rawMsg(id+"@sender.example", "sender@sender.example"))
			if err != nil {
				t.Errorf("writing the message: %v", err)
			}
		case strings.Contains(r.URL.Path, "/mailFolders/sentitems/"):
			writeJSON(w, map[string]any{"value": []any{}, "@odata.deltaLink": srv.URL + r.URL.Path + "?$deltatoken=s2"})
		default:
			m.preferences = append(m.preferences, strings.Join(r.Header.Values("Prefer"), "; "))
			m.page(w, srv.URL+r.URL.Path, r.URL.Query().Get("skip"), r.Header.Values("Prefer"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (m *throttledMailbox) page(w http.ResponseWriter, link, at string, preferences []string) {
	from, err := strconv.Atoi(at)
	if err != nil {
		from = 0
	}
	size := graphPage
	for _, preference := range preferences {
		if asked, err := strconv.Atoi(strings.TrimPrefix(preference, "odata.maxpagesize=")); err == nil && asked < size {
			size = asked
		}
	}
	to := min(from+size, m.messages)
	value := make([]map[string]any, 0, to-from)
	for i := from; i < to; i++ {
		value = append(value, map[string]any{"id": fmt.Sprintf("m%04d", i)})
	}
	body := map[string]any{"value": value}
	if to < m.messages {
		body["@odata.nextLink"] = link + "?skip=" + strconv.Itoa(to)
	} else {
		body["@odata.deltaLink"] = link + "?$deltatoken=i2"
	}
	writeJSON(w, body)
}

// elapsedCheckpoints records the virtual time and the fetch count each
// checkpoint landed at.
type elapsedCheckpoints struct {
	mailbox *throttledMailbox
	at      []time.Duration
	fetched []int
	cursors []connector.Cursor
}

func (c *elapsedCheckpoints) CheckpointSync(_ context.Context, cursor connector.Cursor) error {
	c.at = append(c.at, c.mailbox.elapsed)
	c.fetched = append(c.fetched, c.mailbox.fetches)
	c.cursors = append(c.cursors, cursor)
	return nil
}

func TestASlowThrottledSharedRoundCheckpointsBeforeTheSyncJobTimesOut(t *testing.T) {
	spec, ok := jobs.SpecFor("capture_sync")
	if !ok || spec.Timeout.Fixed <= 0 {
		t.Fatal("the capture_sync job declares no fixed timeout")
	}
	mailbox := &throttledMailbox{messages: 300, throttleAt: 90}
	srv := mailbox.serve(t)
	bundle, err := SealShared(SharedCredential{
		TenantID: "contoso.example", ClientID: "app-1", ClientSecret: "s3cret", Mailbox: sharedAddress,
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, state, err := sharedConnector(srv).OpenShared(bundle)
	if err != nil {
		t.Fatal(err)
	}
	root := srv.URL + "/users/" + sharedAddress + "/mailFolders/"
	cursor, err := marshalCursor(cursorState{
		Email: sharedAddress, DeltaLink: root + "inbox/messages/delta", SentDeltaLink: root + "sentitems/messages/delta",
	})
	if err != nil {
		t.Fatal(err)
	}
	saved := &elapsedCheckpoints{mailbox: mailbox}
	ctx := connector.WithSyncCheckpointer(context.Background(), saved)

	_, err = opened.(*Connector).SyncFrom(ctx, state, cursor, time.Time{}, &recordingSink{})
	var limited *connector.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 30*time.Second {
		t.Fatalf("err = %v, want the 429 with its Retry-After", err)
	}
	if len(saved.at) == 0 || saved.at[0] >= spec.Timeout.Fixed {
		t.Fatalf("first checkpoint at %v of %v virtual time, want one before the %v job timeout", saved.at, mailbox.elapsed, spec.Timeout.Fixed)
	}
	resumed, err := parseCursor(saved.cursors[len(saved.cursors)-1])
	if err != nil || resumed.Rounds[folderInbox] == nil || !strings.Contains(resumed.Rounds[folderInbox].NextLink, "skip=") {
		t.Errorf("the last checkpoint = %+v, %v; want the inbox round standing at a page", resumed, err)
	}
	last := 0
	for _, fetched := range saved.fetched {
		if fetched-last > sharedCheckpointMessages {
			t.Errorf("checkpoints at fetches %v leave a gap over %d messages", saved.fetched, sharedCheckpointMessages)
		}
		last = fetched
	}
	for _, preference := range mailbox.preferences {
		if preference != immutableMessageIDs+"; odata.maxpagesize=25" {
			t.Errorf("a delta page was asked for with Prefer %q, want the immutable ids and pages of 25", preference)
		}
	}
	// One provider page alone would hold more than a span at this latency.
	if page := time.Duration(graphPage) * mimeLatency; page <= spec.Timeout.Fixed {
		t.Errorf("a %d-message page takes %v, inside the timeout; this test no longer shows the cap", graphPage, page)
	}
}

func TestAPersonalRoundStillCheckpointsByPagesAlone(t *testing.T) {
	c := New(nil, nil)
	for pages := 1; pages < checkpointEveryPages; pages++ {
		if c.checkpointDue(pages, 1000) {
			t.Fatalf("a personal round checkpointed after %d pages", pages)
		}
	}
	if !c.checkpointDue(checkpointEveryPages, 0) {
		t.Error("a personal round skipped its page checkpoint")
	}
}

func TestOnlyASharedClientAsksForSmallPages(t *testing.T) {
	var prefers, tops []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefers = append(prefers, strings.Join(r.Header.Values("Prefer"), "; "))
		tops = append(tops, r.URL.Query().Get(paramTop))
		writeJSON(w, map[string]any{"value": []any{}, "id": "f1"})
	}))
	t.Cleanup(srv.Close)
	shared, err := NewSharedAPI(srv.Client(), srv.URL, sharedAddress, sharedUserID)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, api := range []API{NewAPI(srv.Client(), srv.URL), shared} {
		if _, _, err := api.ListAfter(context.Background(), "t", after, "", backfillPageSize); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{immutableMessageIDs, immutableMessageIDs + "; odata.maxpagesize=25"}
	if strings.Join(prefers, "|") != strings.Join(want, "|") || strings.Join(tops, "|") != strconv.Itoa(backfillPageSize)+"|25" {
		t.Errorf("Prefer %q and $top %q; want the personal request unchanged and the shared one capped at 25", prefers, tops)
	}
}
