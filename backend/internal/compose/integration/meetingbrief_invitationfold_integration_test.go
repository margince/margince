// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The brief's history reads a held meeting's invitation mail as the meeting,
// as History does: the invitation copies are not conversations of their own.
func TestTheBriefsHistoryFoldsAMeetingsInvitationMailOntoIt(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	owner := OwnerConn(t)
	attendee := e.SeedContact(t, "Ana Roth", &e.Rep1)
	uid := "kickoff-" + ids.NewV7().String() + "@google.com"
	held := roomAgo(3 * 24 * time.Hour)

	room := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Follow-up call', $2, 'manual', 'human:x')`, roomTomorrow)
	kickoff := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Kickoff', $2, 'held', 'manual', 'human:x')`, held)
	if _, err := owner.Exec(t.Context(), `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'seed')`, activities.MeetingIdentityKey(uid, held.UTC().Format(time.RFC3339)), kickoff); err != nil {
		t.Fatal(err)
	}
	invitation := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by, calendar_method, calendar_uid)
		VALUES ($1, 'email', 'Invitation: Kickoff', 'Join the kickoff.', 'outbound', $2, 'manual', 'human:x', 'REQUEST', $3)`,
		held.Add(-48*time.Hour), uid)
	written := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Re: Invitation: Kickoff', 'Could we start at ten?', 'inbound', $2, 'manual', 'human:x')`,
		held.Add(-24*time.Hour))
	for _, activity := range []ids.UUID{room, kickoff, invitation, written} {
		LinkActivity(t, owner, activity, "contact", attendee)
		seatInRoom(t, owner, e.WS, activity, attendee)
	}

	lane := &planPromptLane{}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	if _, err := meetingBriefService(e).WithLane(lane).Get(rep, room); err != nil {
		t.Fatalf("Get: %v", err)
	}
	prompt := lane.planPrompt(t)
	prompt.message(t, "Re: Invitation: Kickoff")
	for _, moment := range prompt.Moments {
		for _, message := range moment.Messages {
			if message.Subject == "Invitation: Kickoff" {
				t.Error("the brief's history lists the meeting's invitation mail as a conversation of its own")
			}
		}
	}
}
