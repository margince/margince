// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which deal the meeting brief is about, and which deal each conversation in
// its plan prompt is filed under, against a real database.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// planPromptLane records every request and answers each with an empty plan.
type planPromptLane struct {
	mu       sync.Mutex
	requests []model.Request
}

func (l *planPromptLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, req)
	return model.Response{Text: "{}"}, nil
}

// promptMessage is one message of the plan prompt, as the model reads it.
type promptMessage struct {
	ActivityID string `json:"activity_id"`
	OnThisDeal *bool  `json:"on_this_deal"`
	OtherDeal  *struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"other_deal"`
}

// planPrompt is the briefing the plan call sent: its deal and its messages.
type planPrompt struct {
	Deal *struct {
		Name string
	} `json:"deal"`
	Moments []struct {
		Messages []promptMessage `json:"messages"`
	} `json:"account_moments"`
}

func (l *planPromptLane) planPrompt(t *testing.T) planPrompt {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, req := range l.requests {
		if !strings.Contains(req.System, "You prepare a salesperson for one meeting") {
			continue
		}
		content := req.Messages[0].Content
		var prompt planPrompt
		if err := json.Unmarshal([]byte(content[strings.Index(content, "{"):strings.LastIndex(content, "}")+1]), &prompt); err != nil {
			t.Fatalf("the plan prompt is not JSON: %v", err)
		}
		return prompt
	}
	t.Fatalf("the plan call was never made; %d requests recorded", len(l.requests))
	return planPrompt{}
}

func (p planPrompt) message(t *testing.T, activity ids.UUID) promptMessage {
	t.Helper()
	for _, moment := range p.Moments {
		for _, message := range moment.Messages {
			if message.ActivityID == activity.String() {
				return message
			}
		}
	}
	t.Fatalf("the plan prompt carries no message %s", activity)
	return promptMessage{}
}

// seedDealInState writes a deal with the given status on a stage of the same
// meaning, directly, so the fixture names the state it tests.
func seedDealInState(t *testing.T, e *Env, name, status string, closes time.Time) ids.UUID {
	t.Helper()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	probability := map[string]int{"open": 20, "won": 100, "lost": 0}[status]
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `INSERT INTO pipeline (id, name, is_default, position)
			VALUES ($1, $2, false, 91)`, pipeline, name+" pipeline"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
			VALUES ($1, $2, $3, 0, $3, $4)`, stage, pipeline, status, probability); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, expected_close_date,
			closed_at, lost_reason, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $3 = 'open' THEN NULL ELSE now() END,
			        CASE WHEN $3 = 'lost' THEN 'No budget this year' END, 'manual', 'human:x')`,
			deal, name, status, pipeline, stage, closes)
		return err
	})
	if err != nil {
		t.Fatalf("seeding deal %q: %v", name, err)
	}
	return deal
}

// twoDealRoom is a meeting on an open and a lost deal. One mail is on each
// deal, and the same attendee sits in all three.
type twoDealRoom struct {
	meeting, lostMail, openMail ids.UUID
}

func seedTwoDealRoom(t *testing.T, e *Env) twoDealRoom {
	t.Helper()
	owner := OwnerConn(t)
	attendee := e.SeedContact(t, "Ana Roth", &e.Rep1)
	lost := seedDealInState(t, e, "Fleet telematics", "lost", roomFixedNow.AddDate(0, -3, 0))
	open := seedDealInState(t, e, "Hangar booking", "open", roomFixedNow.AddDate(0, 2, 0))

	var room twoDealRoom
	room.meeting = SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Discovery call', $2, 'manual', 'human:x')`, roomTomorrow)
	room.lostMail = SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Telematics decision', 'We will not go ahead with telematics.', 'inbound', $2, 'manual', 'human:x')`,
		roomAgo(20*24*time.Hour))
	room.openMail = SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Hangar slots', 'Could hangar booking replace our spreadsheet?', 'inbound', $2, 'manual', 'human:x')`,
		roomAgo(3*24*time.Hour))
	for _, activity := range []ids.UUID{room.meeting, room.lostMail, room.openMail} {
		LinkActivity(t, owner, activity, "contact", attendee)
		seatInRoom(t, owner, e.WS, activity, attendee)
	}
	LinkActivity(t, owner, room.lostMail, "deal", lost)
	LinkActivity(t, owner, room.openMail, "deal", open)
	// The meeting is filed under both. The lost deal closes earlier, so a
	// close-date order alone would brief on it.
	LinkActivity(t, owner, room.meeting, "deal", lost)
	LinkActivity(t, owner, room.meeting, "deal", open)
	return room
}

func TestAMeetingOnAnOpenAndAClosedDealBriefsOnTheOpenOne(t *testing.T) {
	e := Setup(t)
	room := seedTwoDealRoom(t, e)
	lane := &planPromptLane{}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	if _, err := meetingBriefService(e).WithLane(lane).Get(rep, room.meeting); err != nil {
		t.Fatalf("Get: %v", err)
	}
	prompt := lane.planPrompt(t)
	if prompt.Deal == nil || prompt.Deal.Name != "Hangar booking" {
		t.Fatalf("the brief is about deal %+v, want the open Hangar booking deal", prompt.Deal)
	}
	onThis := prompt.message(t, room.openMail)
	if onThis.OnThisDeal == nil || !*onThis.OnThisDeal || onThis.OtherDeal != nil {
		t.Errorf("the open deal's mail = %+v, want on_this_deal and no other deal", onThis)
	}
	onLost := prompt.message(t, room.lostMail)
	if onLost.OnThisDeal == nil || *onLost.OnThisDeal {
		t.Errorf("the lost deal's mail says on_this_deal = %v, want false", onLost.OnThisDeal)
	}
	if onLost.OtherDeal == nil || onLost.OtherDeal.Name != "Fleet telematics" || onLost.OtherDeal.State != "lost" {
		t.Errorf("the lost deal's mail names other deal %+v, want Fleet telematics, lost", onLost.OtherDeal)
	}
}

func TestHistoryNamesAnotherDealOnlyWhenTheReaderMaySeeIt(t *testing.T) {
	e := Setup(t)
	room := seedTwoDealRoom(t, e)
	noDeals := roomPerms
	noDeals.Objects = map[string]principal.ObjectGrant{}
	for object, grant := range roomPerms.Objects {
		if object != "deal" {
			noDeals.Objects[object] = grant
		}
	}
	lane := &planPromptLane{}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, noDeals)
	if _, err := meetingBriefService(e).WithLane(lane).Get(rep, room.meeting); err != nil {
		t.Fatalf("Get: %v", err)
	}
	prompt := lane.planPrompt(t)
	if prompt.Deal != nil {
		t.Errorf("a reader with no deal grant was briefed on deal %+v", prompt.Deal)
	}
	if onLost := prompt.message(t, room.lostMail); onLost.OtherDeal != nil {
		t.Errorf("a reader with no deal grant was told the mail is on %+v", onLost.OtherDeal)
	}
}

// A meeting filed under no deal is briefed on its contact's leading open deal.
// Mail on that deal is on this meeting's deal, and names no other.
func TestMailOnTheContactsLeadingDealIsOnAnUnlinkedMeetingsDeal(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	attendee := e.SeedContact(t, "Ana Roth", &e.Rep1)
	leading := seedDealInState(t, e, "Hangar booking", "open", roomFixedNow.AddDate(0, 2, 0))
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, attendee, leading)

	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Discovery call', $2, 'manual', 'human:x')`, roomTomorrow)
	mail := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Hangar slots', 'Could hangar booking replace our spreadsheet?', 'inbound', $2, 'manual', 'human:x')`,
		roomAgo(3*24*time.Hour))
	for _, activity := range []ids.UUID{meeting, mail} {
		LinkActivity(t, owner, activity, "contact", attendee)
		seatInRoom(t, owner, e.WS, activity, attendee)
	}
	LinkActivity(t, owner, mail, "deal", leading)

	lane := &planPromptLane{}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	if _, err := meetingBriefService(e).WithLane(lane).Get(rep, meeting); err != nil {
		t.Fatalf("Get: %v", err)
	}
	prompt := lane.planPrompt(t)
	if prompt.Deal == nil || prompt.Deal.Name != "Hangar booking" {
		t.Fatalf("the brief is about deal %+v, want the contact's leading Hangar booking deal", prompt.Deal)
	}
	got := prompt.message(t, mail)
	if got.OnThisDeal == nil || !*got.OnThisDeal || got.OtherDeal != nil {
		t.Errorf("mail on the leading deal = %+v, want on_this_deal and no other deal", got)
	}
}
