// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"maps"
	"reflect"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestStatusCopyCatalog(t *testing.T) { langcopytest.Census(t, statusWords) }

func TestStatusLanguageKeepsTheSameActionAndDismissal(t *testing.T) {
	last := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -10))
	request := inboundMail(testNow.AddDate(0, 0, -3))
	owed := request
	owed.EmailSummary = &crmcontracts.EmailSummary{Move: crmcontracts.EmailSummaryMoveNeedsReply}
	reminded := request
	reminded.EmailSummary = &crmcontracts.EmailSummary{RequestHasReminder: new(true)}
	unnamed := request
	unnamed.Subject = nil
	roleOnly := seat(roleChampion, "Hidden Name")
	roleOnly.Attachable = false
	closed := openDeal()
	closed.Status = crmcontracts.DealStatusWon
	for name, f := range map[string]facts{
		"empty":             {deal: openDeal(), now: testNow, loc: time.UTC},
		"quiet":             {deal: openDeal(), now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}},
		"named meeting":     {deal: openDeal(), now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}, seats: []Seat{seat(roleChampion, "Original Name")}},
		"opening":           {deal: openDeal(), now: testNow, loc: time.UTC, seats: []Seat{seat(roleEconomicBuyer, "Original Name")}},
		"task":              {deal: openDeal(), now: testNow, loc: time.UTC, openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Original task"}}},
		"booked":            {deal: openDeal(), now: testNow, loc: time.UTC, nextMeeting: booked(testNow.AddDate(0, 0, 2))},
		"far meeting":       {deal: openDeal(), now: testNow, loc: time.UTC, nextMeeting: booked(testNow.AddDate(0, 0, 30))},
		"follow up":         {deal: openDeal(), now: testNow, loc: time.UTC, followUps: []activities.AwaitingReply{owedAfter(ids.NewV7())}},
		"request review":    {deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{request}},
		"reply owed":        {deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{owed}},
		"reminder":          {deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{reminded}},
		"unnamed request":   {deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{unnamed}},
		"role-only meeting": {deal: openDeal(), now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}, seats: []Seat{roleOnly}},
		"closed":            {deal: closed, now: testNow, loc: time.UTC},
	} {
		t.Run(name, func(t *testing.T) {
			baseline := decideMove(f)
			english := composeDeterministic(f, baseline)
			for _, language := range textlang.Shipped {
				f.lang = string(language)
				move := decideMove(f)
				if move.Action != baseline.Action || !reflect.DeepEqual(move.Fingerprint, baseline.Fingerprint) {
					t.Fatalf("%s changed the action or dismissal identity", language)
				}
				if language != textlang.English && move.Reason == baseline.Reason {
					t.Fatalf("%s retained an English action reason", language)
				}
				if !reflect.DeepEqual(actionLinks(move), actionLinks(baseline)) {
					t.Fatalf("%s changed the action operands", language)
				}
				if len(move.Evidence) != len(baseline.Evidence) {
					t.Fatal("language changed citation count")
				}
				for i, evidence := range move.Evidence {
					if !reflect.DeepEqual(evidence.ActivityId, baseline.Evidence[i].ActivityId) || !reflect.DeepEqual(evidence.OccurredAt, baseline.Evidence[i].OccurredAt) {
						t.Fatal("language changed the cited record or date")
					}
				}
				card := composeDeterministic(f, move)
				if len(card.Story.Sentences) != len(english.Story.Sentences) {
					t.Fatal("language changed the story facts")
				}
				for i, sentence := range card.Story.Sentences {
					if language != textlang.English && sentence.Text == english.Story.Sentences[i].Text {
						t.Fatalf("%s left story sentence %d in English", language, i)
					}
				}
			}
		})
	}
}

func actionLinks(move crmcontracts.DealStatusCardMove) map[string]any {
	if move.Arguments == nil {
		return nil
	}
	args := maps.Clone(*move.Arguments)
	delete(args, "subject")
	return args
}
