// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The line a buyer reads about what the room records, and when they read it.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
)

const noticeCompany = "Northwind Trading"

func noticeFor(t *testing.T, e *Env, sess dealrooms.Session) (company string, shown bool) {
	t.Helper()
	view, err := dealrooms.NewStore(e.DB()).BuyerView(context.Background(), sess,
		func(context.Context) (string, error) { return noticeCompany, nil })
	if err != nil {
		t.Fatalf("reading the room as the buyer: %v", err)
	}
	if view.EngagementNotice == nil {
		return "", false
	}
	return view.EngagementNotice.Company, true
}

func sessionFromCredential(t *testing.T, e *Env, credential string) dealrooms.Session {
	t.Helper()
	rooms := dealrooms.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)
	issued, err := rooms.ExchangeCredential(ctx, credential)
	if err != nil {
		t.Fatalf("exchanging a credential: %v", err)
	}
	sess, err := rooms.ResolveSession(ctx, issued.Token)
	if err != nil {
		t.Fatalf("resolving the session: %v", err)
	}
	return sess
}

// The first sign-in of a seat carries the notice naming the seller's company,
// and the sign-in it was shown on is the recorded one.
func TestABuyerSeesTheEngagementNoticeOnFirstSignIn(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "notice.first@acme.test")
	sess := signedInBuyer(t, e, buyer)

	company, shown := noticeFor(t, e, sess)
	if !shown || company != noticeCompany {
		t.Fatalf("first sign-in notice = (%q, %v), want it naming %q", company, shown, noticeCompany)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_engagement WHERE participant_id = $1 AND kind = 'signed_in'`, buyer.seat); n != 1 {
		t.Errorf("signed_in rows = %d, want the one sign-in the notice was shown on", n)
	}
}

// A buyer who signs in again, on this device or another, is not told twice.
func TestTheEngagementNoticeDoesNotRepeatOnALaterSignIn(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "notice.again@acme.test")
	first := signedInBuyer(t, e, buyer)
	if _, shown := noticeFor(t, e, first); !shown {
		t.Fatal("first sign-in showed no notice")
	}
	if _, shown := noticeFor(t, e, first); !shown {
		t.Error("a reload of the first session lost its notice before a second sign-in")
	}

	again, err := dealrooms.NewStore(e.DB()).ResendInvitation(e.As(e.AdminUser, nil, roomErasureAdmin), buyer.room, buyer.seat)
	if err != nil {
		t.Fatalf("re-inviting the seat: %v", err)
	}
	second := sessionFromCredential(t, e, again.Credential)
	if company, shown := noticeFor(t, e, second); shown {
		t.Errorf("second sign-in showed the notice for %q again", company)
	}
}

// A seller previewing the room reads what the buyer will read, every time, and
// the preview is not recorded as a buyer arriving.
func TestAPreviewShowsTheEngagementNoticeAndRecordsNothing(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "notice.preview@acme.test")
	rooms := dealrooms.NewStore(e.DB())
	for range 2 {
		issued, err := rooms.PreviewRoom(e.As(e.AdminUser, nil, roomErasureAdmin), buyer.room)
		if err != nil {
			t.Fatalf("previewing the room: %v", err)
		}
		sess := sessionFromCredential(t, e, issued.Credential)
		if _, shown := noticeFor(t, e, sess); !shown {
			t.Error("a preview showed no notice")
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_engagement WHERE room_id = $1`, buyer.room); n != 0 {
		t.Errorf("a preview wrote %d engagement rows, want 0", n)
	}
}

// A paused room serves no content, so it carries no notice either.
func TestAPausedRoomCarriesNoEngagementNotice(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "notice.paused@acme.test")
	sess := signedInBuyer(t, e, buyer)
	if _, err := dealrooms.NewStore(e.DB()).PauseRoom(e.As(e.AdminUser, nil, roomErasureAdmin), buyer.room); err != nil {
		t.Fatalf("pausing the room: %v", err)
	}
	if company, shown := noticeFor(t, e, sess); shown {
		t.Errorf("a paused room showed the notice for %q", company)
	}
}
