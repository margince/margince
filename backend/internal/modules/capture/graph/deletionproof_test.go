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

// deletionMailbox is one Outlook mailbox synced again and again from its
// stored cursor. Every folder already has a watermark, so no sync anchors.
type deletionMailbox struct {
	t      *testing.T
	api    *fakeAPI
	sink   *graphRemovalSink
	cursor connector.Cursor
}

func newDeletionMailbox(t *testing.T) *deletionMailbox {
	t.Helper()
	return &deletionMailbox{
		t:    t,
		api:  &fakeAPI{email: owner, deltaLink: "https://graph/delta?token=2", raws: map[string][]byte{}},
		sink: &graphRemovalSink{},
		cursor: testCursor(t, cursorState{
			DeltaLink: "https://graph/delta?token=1", SentDeltaLink: "https://graph/sent?token=1",
			DeletionsDeltaLink: "https://graph/deletions?token=0", Email: owner,
		}),
	}
}

// day syncs the mailbox n days after pinned, then clears what the provider
// reported, so each event is reported once.
func (m *deletionMailbox) day(n int) {
	m.t.Helper()
	c := pinnedConn(m.api)
	c.now = func() time.Time { return pinned.Add(time.Duration(n) * 24 * time.Hour) }
	cursor, err := c.Sync(context.Background(), authBytes(m.t), m.cursor, m.sink)
	if err != nil {
		m.t.Fatalf("day %d sync: %v", n, err)
	}
	m.cursor = cursor
	m.api.deltaIDs, m.api.deletionsIDs = nil, nil
}

func (m *deletionMailbox) purged() []string {
	var ids []string
	for _, key := range m.sink.removed {
		if key.SourceSystem != connectorName {
			m.t.Errorf("a purge named source system %q", key.SourceSystem)
		}
		ids = append(ids, key.SourceID)
	}
	return ids
}

func (m *deletionMailbox) pending() map[string]pendingPurge {
	m.t.Helper()
	state, err := parseCursor(m.cursor)
	if err != nil {
		m.t.Fatal(err)
	}
	return state.PendingPurges
}

func TestAHardDeletedMessageIsPurgedOnlyAfterTheHold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		arriveDay int
	}{
		{"Shift+Delete moves it straight to Deletions", 0},
		{"Delete, then emptying Deleted Items five days later", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDeletionMailbox(t)
			// Delete alone is a move to Deleted Items; Graph shows it only as a
			// folder tombstone, which the client reports as nothing.
			m.day(0)
			m.api.deletionsIDs = []string{"m1"}
			m.api.whereIs = map[string]string{"m1": deletionsFolderID}
			m.day(tc.arriveDay)
			m.day(tc.arriveDay + 13)
			if got := m.purged(); len(got) != 0 {
				t.Fatalf("purged %v before the hold ran out", got)
			}
			m.day(tc.arriveDay + 14)
			if got := m.purged(); len(got) != 1 || got[0] != "m1" {
				t.Fatalf("purged %v, want m1 once the hold ran out", got)
			}
			m.day(tc.arriveDay + 15)
			if got := m.purged(); len(got) != 1 || len(m.pending()) != 0 {
				t.Fatalf("purged %v with %v still pending, want one purge and none pending", got, m.pending())
			}
		})
	}
}

func TestAMessageThatNeverReachesDeletionsIsNeverPurged(t *testing.T) {
	for _, tc := range []struct{ name, whereIs string }{
		{"a move between ordinary folders", "folder-projects"},
		{"a move to the Online Archive, where the id stops resolving", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDeletionMailbox(t)
			if tc.whereIs != "" {
				m.api.whereIs = map[string]string{"m1": tc.whereIs}
			}
			m.day(0)
			m.day(30)
			if got := m.purged(); len(got) != 0 {
				t.Fatalf("purged %v without a Deletions arrival", got)
			}
			if len(m.api.lookedUp) != 0 {
				t.Errorf("looked up %v; an absence must never be asked about as proof", m.api.lookedUp)
			}
		})
	}
}

func TestARestoreWithinTheHoldKeepsTheMessage(t *testing.T) {
	t.Run("restored to a synced folder", func(t *testing.T) {
		m := newDeletionMailbox(t)
		m.api.deletionsIDs = []string{"m1"}
		m.day(0)
		m.api.deltaIDs = []string{"m1"}
		m.api.raws["m1"] = rawMsg("m1@mail.example", "alice@acme.com")
		m.day(3)
		if len(m.pending()) != 0 {
			t.Fatalf("pending %v after the message came back", m.pending())
		}
		m.day(20)
		if got := m.purged(); len(got) != 0 {
			t.Fatalf("purged %v the owner restored", got)
		}
	})
	t.Run("restored to a folder capture does not read", func(t *testing.T) {
		m := newDeletionMailbox(t)
		m.api.deletionsIDs = []string{"m1"}
		m.day(0)
		m.api.whereIs = map[string]string{"m1": "folder-projects"}
		m.day(14)
		if got := m.purged(); len(got) != 0 {
			t.Fatalf("purged %v the owner restored", got)
		}
		if len(m.pending()) != 0 {
			t.Errorf("pending %v after the lookup found it restored", m.pending())
		}
	})
}

func TestAPendingMessageWhoseIDStopsResolvingIsPurgedOnlyWhenFoundInDeletions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		copiesIn []string
		findErr  error
		purged   bool
		pending  bool
	}{
		{"restored, then moved to the Online Archive under a new id", []string{"folder-archive"}, nil, false, false},
		{"one copy in Deletions and one kept in Inbox", []string{deletionsFolderID, inboxFolderID}, nil, false, false},
		{"found only in Deletions under a new id", []string{deletionsFolderID}, nil, true, false},
		{"found nowhere, as a restore to the Online Archive reads", nil, nil, false, false},
		{"the search fails", nil, ErrUnreachable, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDeletionMailbox(t)
			m.api.deletionsIDs = []string{"m1"}
			m.day(0)
			if _, held := m.pending()["m1"]; !held {
				t.Fatalf("pending %v, want m1 held from its arrival", m.pending())
			}
			m.api.copiesOf = map[string][]string{internetIDOf("m1"): tc.copiesIn}
			m.api.findMessageErr = tc.findErr
			m.day(14)
			if got := m.purged(); (len(got) == 1) != tc.purged {
				t.Fatalf("purged %v, want purged=%v", got, tc.purged)
			}
			if _, held := m.pending()["m1"]; held != tc.pending {
				t.Fatalf("pending %v, want m1 pending=%v", m.pending(), tc.pending)
			}
			if len(m.api.searchedFor) != 1 || m.api.searchedFor[0] != internetIDOf("m1") {
				t.Errorf("searched for %v, want the identity seen on arrival", m.api.searchedFor)
			}
		})
	}
}

func TestAPendingMessageWithNoInternetIDIsKeptOnA404(t *testing.T) {
	m := newDeletionMailbox(t)
	m.cursor = testCursor(t, cursorState{
		DeltaLink: "d", SentDeltaLink: "s", DeletionsDeltaLink: "x", Email: owner,
		PendingPurges: map[string]pendingPurge{"m1": {Seen: pinned}},
	})
	m.day(14)
	if got := m.purged(); len(got) != 0 || len(m.pending()) != 0 {
		t.Fatalf("purged %v with %v pending, want m1 kept and its hold dropped", got, m.pending())
	}
}

func TestPendingPurgesReadFromTheCursorAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pending map[string]pendingPurge
		kept    map[string]bool
	}{
		{
			"an entry with no time or a time after now is dropped",
			map[string]pendingPurge{"future": {Seen: pinned.Add(time.Hour)}, "zero": {}, "m1": {Seen: pinned}},
			map[string]bool{"future": false, "zero": false, "m1": true},
		},
		{"past the cap the oldest are kept", pendingAtSeconds(pendingPurgeCap + 1), map[string]bool{"id-0": false, "id-1": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDeletionMailbox(t)
			m.cursor = testCursor(t, cursorState{DeltaLink: "d", SentDeltaLink: "s", DeletionsDeltaLink: "x", Email: owner, PendingPurges: tc.pending})
			m.day(0)
			if got := len(m.pending()); got > pendingPurgeCap {
				t.Fatalf("kept %d purges, past the cap %d", got, pendingPurgeCap)
			}
			for id, want := range tc.kept {
				if _, held := m.pending()[id]; held != want {
					t.Errorf("%s held=%v, want %v", id, held, want)
				}
			}
			if len(m.api.lookedUp) != 0 {
				t.Errorf("looked up %v; no entry was due", m.api.lookedUp)
			}
		})
	}
}

// pendingAtSeconds holds n entries; id-i arrived i seconds before pinned.
func pendingAtSeconds(n int) map[string]pendingPurge {
	pending := map[string]pendingPurge{}
	for i := range n {
		pending[fmt.Sprintf("id-%d", i)] = pendingPurge{Seen: pinned.Add(-time.Duration(i) * time.Second)}
	}
	return pending
}

func TestAMessageStillInDeletionsWhenTheHoldEndsIsPurged(t *testing.T) {
	m := newDeletionMailbox(t)
	m.api.deletionsIDs = []string{"m1"}
	m.day(0)
	m.api.whereIs = map[string]string{"m1": deletionsFolderID}
	m.day(14)
	if got := m.purged(); len(got) != 1 || got[0] != "m1" {
		t.Fatalf("purged %v, want m1", got)
	}
}

func TestAFailedLookupKeepsThePurgePending(t *testing.T) {
	m := newDeletionMailbox(t)
	m.api.deletionsIDs = []string{"m1"}
	m.day(0)
	m.api.lookupErrIDs = map[string]error{"m1": ErrUnreachable}
	m.day(14)
	if got := m.purged(); len(got) != 0 || len(m.pending()) != 1 {
		t.Fatalf("purged %v with %v pending after a failed lookup, want nothing purged and m1 pending", got, m.pending())
	}
	m.api.lookupErrIDs = nil
	m.api.whereIs = map[string]string{"m1": deletionsFolderID}
	m.day(15)
	if got := m.purged(); len(got) != 1 {
		t.Fatalf("purged %v once the lookup answered, want m1", got)
	}
}

func TestTheFirstDeletionsRoundOnlySetsTheBaseline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start cursorState
		err   error
	}{
		{"a mailbox with no Deletions watermark", cursorState{DeltaLink: "d", SentDeltaLink: "s", Email: owner}, nil},
		{"a watermark Graph no longer honours", cursorState{DeltaLink: "d", SentDeltaLink: "s", DeletionsDeltaLink: "old", Email: owner}, ErrDeltaGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newDeletionMailbox(t)
			m.cursor = testCursor(t, tc.start)
			m.api.deletionsErr = tc.err
			m.api.deletionsIDs = []string{"deleted-long-ago"}
			m.day(0)
			if len(m.pending()) != 0 {
				t.Fatalf("held %v from what the folder already held", m.pending())
			}
			state, err := parseCursor(m.cursor)
			if err != nil || state.DeletionsDeltaLink == "" {
				t.Fatalf("no Deletions watermark stored: %+v %v", state, err)
			}
		})
	}
}

func TestAFailingDeletionsDeltaFallsBackToABoundedListing(t *testing.T) {
	m := newDeletionMailbox(t)
	m.api.deletionsErr = ErrUnreachable
	m.api.listedDeletions = []string{"old"}
	m.day(0)
	if len(m.pending()) != 0 || m.api.listDeletionsCalls != 1 {
		t.Fatalf("the first listing held %v after %d listings, want a baseline", m.pending(), m.api.listDeletionsCalls)
	}
	m.api.listedDeletions = []string{"old", "m1"}
	m.day(1)
	if _, held := m.pending()["m1"]; !held || len(m.pending()) != 1 {
		t.Fatalf("pending %v, want only the new arrival m1", m.pending())
	}
	m.api.whereIs = map[string]string{"m1": deletionsFolderID}
	m.day(15)
	if got := m.purged(); len(got) != 1 || got[0] != "m1" {
		t.Fatalf("purged %v, want m1", got)
	}
}

func TestAnUnreadableDeletionsFolderDoesNotStopMailCapture(t *testing.T) {
	m := newDeletionMailbox(t)
	m.api.deletionsErr = ErrUnreachable
	m.api.listDeletionsErr = ErrUnreachable
	m.api.deltaIDs = []string{"m2"}
	m.api.raws["m2"] = rawMsg("m2@mail.example", "bob@acme.com")
	m.day(0)
	if len(m.sink.recs) == 0 {
		t.Fatal("mail capture stopped over the Deletions folder")
	}
	state, err := parseCursor(m.cursor)
	if err != nil || state.DeletionsDeltaLink != "https://graph/deletions?token=0" {
		t.Fatalf("the Deletions watermark moved after a failed read: %+v %v", state, err)
	}
}

func TestPendingPurgesStopAtTheirCap(t *testing.T) {
	var state cursorState
	for i := range pendingPurgeCap + 1 {
		state.holdPurge(MessageRef{ID: fmt.Sprintf("id-%d", i)}, pinned)
	}
	if len(state.PendingPurges) != pendingPurgeCap {
		t.Fatalf("held %d purges, want the cap %d", len(state.PendingPurges), pendingPurgeCap)
	}
	state.holdPurge(MessageRef{ID: "id-0"}, pinned.Add(time.Hour))
	if !state.PendingPurges["id-0"].Seen.Equal(pinned) {
		t.Fatal("a second arrival restarted the hold")
	}
}

func TestAFailedPurgeFailsTheSync(t *testing.T) {
	m := newDeletionMailbox(t)
	m.api.deletionsIDs = []string{"m1"}
	m.day(0)
	m.api.whereIs = map[string]string{"m1": deletionsFolderID}
	m.sink.err = errors.New("the database refused")
	c := pinnedConn(m.api)
	c.now = func() time.Time { return pinned.Add(deletionHold) }
	if _, err := c.Sync(context.Background(), authBytes(t), m.cursor, m.sink); err == nil {
		t.Fatal("a failed purge was reported as a clean sync")
	}
}
