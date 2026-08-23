// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package nextaction

// The model lane, driven through the same answer path production runs: a
// grounded proposal replaces the fallback task, everything else — an erroring
// lane, an ungrounded citation, an id in reader text — serves the
// deterministic fallback unchanged, and generated_by says which happened.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// laneReturning is the model seam scripted: one reply, or one error, and a
// record of what was sent so a test can hold the request's own properties.
type laneReturning struct {
	reply string
	err   error
	sent  []model.Request
}

func (l *laneReturning) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.sent = append(l.sent, req)
	if l.err != nil {
		return model.Response{}, l.err
	}
	return model.Response{Text: l.reply}, nil
}

func fallbackFacts() facts {
	past := outbound(now.Add(-9 * 24 * time.Hour))
	return facts{
		deal:     crmcontracts.Deal{Name: "Kessler GmbH — WMS Pilot", Status: crmcontracts.DealStatusOpen},
		timeline: []crmcontracts.Activity{past},
		now:      now,
	}
}

func serviceWith(lane Completer) *Service {
	s := NewService(nil, nil, func() time.Time { return now })
	if lane != nil {
		s = s.WithLane(lane)
	}
	return s
}

func TestAGroundedProposalReplacesTheFallbackTask(t *testing.T) {
	f := fallbackFacts()
	cited := f.timeline[0].Id.String()
	lane := &laneReturning{reply: fmt.Sprintf(
		`{"subject":"Send Kessler the revised pilot offer","reason":"They asked for it on the last call and nothing went out.","evidence":[%q]}`, cited)}

	out := serviceWith(lane).answer(context.Background(), f)

	if out.Action != ActionCreateTask {
		t.Fatalf("action = %q; the lane may never move the verb", out.Action)
	}
	if got := (*out.Arguments)["subject"]; got != "Send Kessler the revised pilot offer" {
		t.Errorf("task subject = %v, want the lane's proposal", got)
	}
	if !strings.Contains(out.Reason, "nothing went out") {
		t.Errorf("reason = %q, want the lane's why", out.Reason)
	}
	if out.GeneratedBy == nil || *out.GeneratedBy != crmcontracts.Model {
		t.Errorf("generated_by = %v, want model", out.GeneratedBy)
	}
	// The floor's links and source survive: the model writes what the task
	// SAYS, never what clicking it does.
	if _, ok := (*out.Arguments)["links"]; !ok {
		t.Error("the proposal dropped the task's deal link")
	}
	if len(lane.sent) != 1 {
		t.Fatalf("lane calls = %d, want exactly 1", len(lane.sent))
	}
	if !strings.Contains(lane.sent[0].Messages[0].Content, "untrusted-") {
		t.Error("the timeline reached the model outside a nonce fence")
	}
}

func TestARefusedOrFailedLaneServesTheDeterministicFallback(t *testing.T) {
	f := fallbackFacts()
	strangerID := "01a00000-0000-7000-8000-000000000000"
	for name, lane := range map[string]*laneReturning{
		"lane error":         {err: errors.New("over budget")},
		"unparseable":        {reply: "Sure! Here is my suggestion:"},
		"empty task":         {reply: `{"subject":"","reason":""}`},
		"uncited proposal":   {reply: `{"subject":"Call them","reason":"Because.","evidence":[]}`},
		"invented citation":  {reply: fmt.Sprintf(`{"subject":"Call them","reason":"Because.","evidence":[%q]}`, strangerID)},
		"id in reader text":  {reply: fmt.Sprintf(`{"subject":"Call them about %s","reason":"Because.","evidence":[%q]}`, f.timeline[0].Id.String(), f.timeline[0].Id.String())},
		"past the size caps": {reply: fmt.Sprintf(`{"subject":%q,"reason":"Because.","evidence":[%q]}`, strings.Repeat("x", 200), f.timeline[0].Id.String())},
	} {
		out := serviceWith(lane).answer(context.Background(), f)
		floor := decide(f)
		if out.Reason != floor.Reason {
			t.Errorf("%s: reason = %q, want the deterministic fallback %q", name, out.Reason, floor.Reason)
		}
		if out.GeneratedBy == nil || *out.GeneratedBy != crmcontracts.Deterministic {
			t.Errorf("%s: generated_by = %v, want deterministic", name, out.GeneratedBy)
		}
	}
}

func TestARuleMatchedAnswerNeverConsultsTheLane(t *testing.T) {
	lane := &laneReturning{reply: `{"subject":"x","reason":"y"}`}
	f := fallbackFacts()
	f.timeline = append([]crmcontracts.Activity{booked(now.Add(24 * time.Hour))}, f.timeline...)

	out := serviceWith(lane).answer(context.Background(), f)

	if out.Action != ActionOpenMeetingBrief {
		t.Fatalf("action = %q, want the booked-meeting rule to win", out.Action)
	}
	if len(lane.sent) != 0 {
		t.Errorf("the lane was consulted %d times on a rule-matched answer", len(lane.sent))
	}
	if out.GeneratedBy == nil || *out.GeneratedBy != crmcontracts.Deterministic {
		t.Errorf("generated_by = %v, want deterministic on a rule-matched answer", out.GeneratedBy)
	}
}

func TestNoLaneMeansTheDeterministicAnswerStamped(t *testing.T) {
	f := fallbackFacts()
	out := serviceWith(nil).answer(context.Background(), f)
	if out.GeneratedBy == nil || *out.GeneratedBy != crmcontracts.Deterministic {
		t.Errorf("generated_by = %v, want deterministic with no lane wired", out.GeneratedBy)
	}
}

func TestAnEmptyTimelineProposalNeedsNoCitation(t *testing.T) {
	f := fallbackFacts()
	f.timeline = nil
	lane := &laneReturning{reply: `{"subject":"Book the first scope conversation for the WMS pilot","reason":"The deal is qualified and nothing has happened yet.","evidence":[]}`}

	out := serviceWith(lane).answer(context.Background(), f)

	if out.GeneratedBy == nil || *out.GeneratedBy != crmcontracts.Model {
		t.Fatalf("generated_by = %v, want model: an empty timeline owes no citation", out.GeneratedBy)
	}
	if got := (*out.Arguments)["subject"]; got != "Book the first scope conversation for the WMS pilot" {
		t.Errorf("task subject = %v, want the lane's proposal", got)
	}
}

func TestAWithheldRowContributesItsDateAndNoWords(t *testing.T) {
	f := fallbackFacts()
	body := "the content this reader may not open"
	subject := "withheld subject"
	held := crmcontracts.ActivityContentStateWithheld
	f.timeline[0].Body = &body
	f.timeline[0].Subject = &subject
	f.timeline[0].ContentState = &held

	in := NextMoveInput(f)

	if len(in.Timeline) != 1 {
		t.Fatalf("timeline rows = %d, want the withheld row to keep its place", len(in.Timeline))
	}
	if in.Timeline[0].Subject != "" || in.Timeline[0].Excerpt != "" {
		t.Errorf("a withheld row's words reached the prompt: %+v", in.Timeline[0])
	}
}
