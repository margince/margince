// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"
	"time"
)

func TestCalendarSyncKeepsInternalNotesOnTheWorkedMeeting(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	_, guest := p.loseRace(t, start)
	p.workedByHand(t, guest)
	p.exec(t, `UPDATE activity SET meeting_notes='Buyer approved the renewal' WHERE id=$1`, guest)
	p.sync(t, "b")
	if survivor := p.wantOneMeeting(t, start); survivor != guest {
		t.Fatal("sync replaced the meeting with notes")
	}
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE id='`+guest.String()+`' AND meeting_notes='Buyer approved the renewal'`); got != 1 {
		t.Fatal("sync lost the internal notes")
	}
}

func TestCalendarFoldingKeepsNotesWithoutRelyingOnAHumanAuditActor(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	_, guest := p.loseRace(t, start)
	p.exec(t, `UPDATE activity SET meeting_notes='Rep notes entered through MCP' WHERE id=$1`, guest)
	p.sync(t, "b")
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE id='`+guest.String()+`' AND archived_at IS NULL AND meeting_notes='Rep notes entered through MCP'`); got != 1 {
		t.Fatal("automatic folding hid the meeting's internal notes")
	}
}
