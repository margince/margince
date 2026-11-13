// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The readers that hand mail to a model or an agent show a mail two
// mailboxes captured once. They are the deal status card, the meeting brief's
// history and the activity datasource. The datasource also shows each reader
// their own copy.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// bothCopies is the mail in two shared mailboxes, each copy filed under entity
// by the seat that captured it, and each seat's own copy.
func bothCopies(t *testing.T, entityType string, entity func(seatCopyEnv) ids.UUID) (seatCopyEnv, map[ids.UUID]ids.UUID) {
	t.Helper()
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
	s.first(t)
	s.second(t)
	rows := capturedMailRows(t, s.e)
	under := entity(s)
	fileCopyUnder(t, s, s.e.Rep1, rows[0].ID, entityType, under)
	fileCopyUnder(t, s, s.e.Rep3, rows[1].ID, entityType, under)
	return s, map[ids.UUID]ids.UUID{s.e.Rep1: rows[0].ID, s.e.Rep3: rows[1].ID}
}

func TestTheDealStatusCardHandsTheModelTheMailOnce(t *testing.T) {
	var deal ids.DealID
	s, own := bothCopies(t, "deal", func(s seatCopyEnv) ids.UUID {
		deal = seedDeals(t, s, "Telematics renewal")[0]
		return deal.UUID
	})
	for reader := range own {
		lane := &promptLane{}
		now := func() time.Time { return captureNow.Add(24 * time.Hour) }
		svc := dealstatus.NewService(s.e.Pool, deals.NewStore(s.e.DB(), installseam.Deals()), activities.NewStore(s.e.DB()),
			dealrooms.NewStore(s.e.DB()), now).WithLane(lane, "seat-copies")
		if _, err := svc.Get(briefReader(s.e, reader), deal, true); err != nil {
			t.Fatalf("reading the deal card as %s: %v", reader, err)
		}
		assertPromptHoldsTheMailOnce(t, "deal card", lane, reader)
	}
}

func TestTheMeetingBriefHistoryHandsTheModelTheMailOnce(t *testing.T) {
	s, own := bothCopies(t, "contact", func(s seatCopyEnv) ids.UUID { return s.contact.UUID })
	meeting := seedMeetingWithSender(t, s)
	for reader := range own {
		assertPromptHoldsTheMailOnce(t, "meeting brief", briefPrompt(t, s, reader, meeting), reader)
	}
}

// A copy the brief shows stands for every copy the reader may open. The mail
// is on the brief's deal when any of them is filed there.
func TestTheMeetingBriefReadsDealFilingOverEveryCopy(t *testing.T) {
	s, own := bothCopies(t, "contact", func(s seatCopyEnv) ids.UUID { return s.contact.UUID })
	filed := seedDeals(t, s, "Fleet telematics", "Hangar booking")
	other, briefed := filed[0], filed[1]
	meeting := seedMeetingWithSender(t, s)
	fileCopyUnder(t, s, s.e.Rep1, meeting, "deal", briefed.UUID)
	// The second seat is shown its own copy, which is filed under the other deal.
	fileCopyUnder(t, s, s.e.Rep3, own[s.e.Rep3], "deal", other.UUID)
	fileCopyUnder(t, s, s.e.Rep1, own[s.e.Rep1], "deal", briefed.UUID)
	prompt := strings.Join(briefPrompt(t, s, s.e.Rep3, meeting).texts(), "")
	if !strings.Contains(prompt, `"on_this_deal":true`) || strings.Contains(prompt, `"on_this_deal":false`) {
		t.Errorf("the mail one copy files under the briefed deal should be on it, got prompt %s", prompt)
	}
}

// seedMeetingWithSender is a meeting with the mail's sender in the room.
func seedMeetingWithSender(t *testing.T, s seatCopyEnv) ids.UUID {
	t.Helper()
	meeting := s.e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Renewal review', $2, 'manual', 'human:x')`, captureNow.Add(48*time.Hour))
	if _, err := s.e.Owner.Exec(context.Background(), `INSERT INTO activity_participant (activity_id, role, contact_id)
		VALUES ($1, 'attendee', $2)`, meeting, s.contact.UUID); err != nil {
		t.Fatalf("seating the sender in the meeting: %v", err)
	}
	fileCopyUnder(t, s, s.e.Rep1, meeting, "contact", s.contact.UUID)
	return meeting
}

// briefPrompt reads the meeting brief as reader and answers what the model was asked.
func briefPrompt(t *testing.T, s seatCopyEnv, reader, meeting ids.UUID) *promptLane {
	t.Helper()
	lane := &promptLane{}
	svc := meetingbrief.NewService(s.e.Pool, contactPage(s.e), contacts.NewStore(s.e.DB()),
		func() time.Time { return captureNow.Add(24 * time.Hour) }).WithLane(lane)
	if _, err := svc.Get(briefReader(s.e, reader), meeting); err != nil {
		t.Fatalf("reading the meeting brief as %s: %v", reader, err)
	}
	return lane
}

// The context graph walks every copy the reader may open, wherever it is
// filed, and none they may not.
func TestTheContextGraphWalksEveryReadableCopy(t *testing.T) {
	for name, c := range map[string]struct {
		postures seatCopyPostures
		reader   func(seatCopyEnv) ids.UUID
		listed   bool
		// body, when set, makes the second copy another message reusing the Message-ID.
		body string
	}{
		"a reader of a forgery": {
			seatCopyPostures{first: true, second: true},
			func(s seatCopyEnv) ids.UUID { return s.e.Rep1 }, false,
			"our bank details changed, pay to the new account",
		},
		"a reader of both copies":         {seatCopyPostures{first: true, second: true}, func(s seatCopyEnv) ids.UUID { return s.e.Rep1 }, true, ""},
		"a reader of the first copy only": {seatCopyPostures{first: true, second: false}, func(s seatCopyEnv) ids.UUID { return s.e.Rep1 }, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSeatCopyEnv(t, c.postures)
			s.first(t)
			if c.body != "" {
				s.secondFrom(t, seatCopySender, "Telematics renewal", c.body)
			} else {
				s.second(t)
			}
			rows := capturedMailRows(t, s.e)
			deal := seedDeals(t, s, "Hangar booking")[0]
			fileCopyUnder(t, s, s.e.Rep1, rows[0].ID, "contact", s.contact.UUID)
			fileCopyUnder(t, s, s.e.Rep3, rows[1].ID, "deal", deal.UUID)
			picture, err := search.NewRetriever(s.e.Store, nil).AssembleContext(briefReader(s.e, c.reader(s)),
				datasource.EntityRef{Type: datasource.EntityContact, ID: s.contact.UUID}, retrieval.AssembleOptions{MaxItems: 10})
			if err != nil {
				t.Fatalf("assembling the sender's context: %v", err)
			}
			listed := false
			for _, section := range picture.Sections {
				for _, item := range section.Items {
					listed = listed || (section.Name == "related_deals" && item.Ref.ID == deal.UUID)
				}
			}
			if listed != c.listed {
				t.Errorf("related_deals lists the deal only the second copy is filed under: %v, want %v", listed, c.listed)
			}
		})
	}
}

func TestAThreadReadThroughTheDatasourceShowsOneCopy(t *testing.T) {
	for name, postures := range map[string]seatCopyPostures{
		"both shared": {first: true, second: true}, "second private": {first: true, second: false},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSeatCopyEnv(t, postures)
			s.first(t)
			s.second(t)
			rows := capturedMailRows(t, s.e)
			readers := map[ids.UUID]ids.UUID{s.e.Rep1: rows[0].ID, s.e.Rep3: rows[1].ID}
			for reader, want := range readers {
				thread := ""
				for _, row := range emailTimeline(t, s.e, reader, true) {
					if row.ThreadKey != nil {
						thread = *row.ThreadKey
					}
				}
				if thread == "" {
					t.Fatalf("the captured mail carries no thread for %s", reader)
				}
				found, _, _, err := activities.NewProvider(s.e.DB()).SearchEntity(readerCtx(s.e, reader),
					datasource.EntityActivity, nil, 10, nil, map[string]string{"thread_key": thread})
				if err != nil {
					t.Fatalf("reading the thread as %s: %v", reader, err)
				}
				var readable []ids.UUID
				for _, record := range found {
					if record.Ref.ID == rows[0].ID || record.Ref.ID == rows[1].ID {
						readable = append(readable, record.Ref.ID)
					}
				}
				if want == rows[0].ID && !postures.second {
					// The first seat cannot open the second copy, which stays its own row.
					readable = without(readable, rows[1].ID)
				}
				if len(readable) != 1 || readable[0] != want {
					t.Errorf("the thread as %s lists copies %v, want only %s", reader, readable, want)
				}
			}
		})
	}
}

// assertPromptHoldsTheMailOnce checks that every request the model was sent
// holds the mail's text once. A prompt names rows by short refs, so which copy
// it chose is the grouping's own test; this one proves the grouping ran.
func assertPromptHoldsTheMailOnce(t *testing.T, surface string, lane *promptLane, reader ids.UUID) {
	t.Helper()
	prompts := lane.texts()
	if len(prompts) == 0 {
		t.Fatalf("%s as %s asked the model nothing", surface, reader)
	}
	for i, prompt := range prompts {
		if got := strings.Count(prompt, seatCopyBody); got > 1 {
			t.Errorf("%s as %s: request %d holds the mail %d times, want once", surface, reader, i, got)
		}
	}
	if !strings.Contains(strings.Join(prompts, ""), seatCopyBody) {
		t.Errorf("%s as %s: no request holds the mail", surface, reader)
	}
}

// promptLane records every request and answers nothing usable, so each
// surface falls back to its own floor after the model was asked.
type promptLane struct {
	mu       sync.Mutex
	requests []model.Request
}

func (l *promptLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, req)
	return model.Response{}, errors.New("the test lane answers nothing")
}

func (l *promptLane) texts() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.requests))
	for _, req := range l.requests {
		out = append(out, fmt.Sprintf("%+v", req.Messages))
	}
	return out
}

func without(list []ids.UUID, drop ids.UUID) []ids.UUID {
	out := list[:0]
	for _, id := range list {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}

// seedDeals are open deals in one new pipeline, owned by the first seat.
func seedDeals(t *testing.T, s seatCopyEnv, names ...string) []ids.DealID {
	t.Helper()
	pipeline := s.e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`)
	open := s.e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	owner := ids.From[ids.UserKind](s.e.Rep1)
	out := make([]ids.DealID, 0, len(names))
	for _, name := range names {
		created, err := deals.NewStore(s.e.DB(), installseam.Deals()).CreateDeal(s.e.AsFullUser(), deals.CreateDealInput{
			Name: name, PipelineID: ids.From[ids.PipelineKind](pipeline), StageID: ids.From[ids.StageKind](open),
			OwnerID: &owner, Source: "manual",
		})
		if err != nil {
			t.Fatalf("creating deal %q: %v", name, err)
		}
		out = append(out, ids.From[ids.DealKind](ids.UUID(created.Id)))
	}
	return out
}

// briefReader is a seat that may read what a meeting brief reads.
func briefReader(e *integration.SearchEnv, user ids.UUID) context.Context {
	read := principal.ObjectGrant{Read: true}
	return principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
				"activity": read, "contact": read, "company": read, "project": read, "deal": read,
				"relationship": read, "installation_settings": read,
			}, RowScope: principal.RowScopeAll},
		})
}
