// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"
)

// LastTouchFor judges a direction's newest rows first. When all of them are
// closed to the reader, the answer is still the newest row the reader can open.
// The reader who opens them all still gets the newest. The closed rows
// outnumber any first window the read may use.
func TestTheNewestReadableReplyIsFoundBehindManyTheReaderCannotOpen(t *testing.T) {
	f := newCorrespondenceFixture(t)
	older := f.logMail(t, "Re: the first proposal", "inbound", 20*24*time.Hour)
	olderAt := roomAgo(20 * 24 * time.Hour)
	const closed = 40
	for day := range closed {
		f.inward = f.logMail(t, "Re: a limited thread", "inbound", time.Duration(closed-day)*time.Hour)
		f.setAudience(t, "participants")
	}
	newestAt := roomAgo(time.Hour)

	blair := f.worklistTouch(t, f.blair)
	if blair.InboundAt == nil || !blair.InboundAt.Equal(olderAt) || blair.InboundID == nil || *blair.InboundID != older {
		t.Errorf("Blair's last inbound = %v (%v), want the reply Blair can open at %v (%v)", blair.InboundAt, blair.InboundID, olderAt, older)
	}
	if page := f.page(t, f.blair); !sameID(page.LastInboundActivityId, older) {
		t.Errorf("Blair's page names inbound %v, want %v as the Worklist does", page.LastInboundActivityId, older)
	}
	alex := f.worklistTouch(t, f.alex)
	if alex.InboundAt == nil || !alex.InboundAt.Equal(newestAt) || alex.InboundID == nil || *alex.InboundID != f.inward {
		t.Errorf("Alex's last inbound = %v, want the newest reply at %v", alex.InboundAt, newestAt)
	}
	if alex.PrecedingContactAt == nil || !alex.PrecedingContactAt.Equal(roomAgo(2*time.Hour)) {
		t.Errorf("Alex's touch before the newest reply = %v, want the reply an hour earlier", alex.PrecedingContactAt)
	}
	if blair.PrecedingContactAt == nil || !blair.PrecedingContactAt.Equal(roomAgo(30*24*time.Hour)) {
		t.Errorf("Blair's touch before the readable reply = %v, want the outbound proposal", blair.PrecedingContactAt)
	}
}

// The outbound clock falls back the same way. Sends the reader cannot open,
// newer than one they can, leave that one as their last send.
func TestTheNewestReadableSendIsFoundBehindManyTheReaderCannotOpen(t *testing.T) {
	f := newCorrespondenceFixture(t)
	for hour := range 40 {
		f.inward = f.logMail(t, "Following up", "outbound", time.Duration(40-hour)*time.Hour)
		f.setAudience(t, "participants")
	}
	blair := f.worklistTouch(t, f.blair)
	if blair.OutboundAt == nil || !blair.OutboundAt.Equal(roomAgo(30*24*time.Hour)) || blair.OutboundID == nil || *blair.OutboundID != f.outbound {
		t.Errorf("Blair's last send = %v (%v), want the proposal Blair can open (%v)", blair.OutboundAt, blair.OutboundID, f.outbound)
	}
	if alex := f.worklistTouch(t, f.alex); alex.OutboundID == nil || *alex.OutboundID != f.inward {
		t.Errorf("Alex's last send = %v, want the newest follow-up %v", alex.OutboundID, f.inward)
	}
}

// Two readable replies share one instant behind a full window of closed ones.
// The answer is the one the whole history picks, never whichever of the pair
// a window happened to keep.
func TestATieAtTheWindowsEdgeAnswersAsTheWholeHistory(t *testing.T) {
	f := newCorrespondenceFixture(t)
	f.logMail(t, "Re: first of two", "inbound", 3*24*time.Hour)
	second := f.logMail(t, "Re: second of two", "inbound", 3*24*time.Hour)
	for hour := range 15 {
		f.inward = f.logMail(t, "Re: a limited thread", "inbound", time.Duration(15-hour)*time.Hour)
		f.setAudience(t, "participants")
	}
	if blair := f.worklistTouch(t, f.blair); blair.InboundID == nil || *blair.InboundID != second {
		t.Errorf("Blair's last inbound = %v, want %v, the later of the tied pair by the whole-history order", blair.InboundID, second)
	}
}
