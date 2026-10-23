// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for summarize/deal_room_brief. It calls
// roombrief.BriefRequest and roombrief.ParseBrief, as the service does.
//
// A reply saying nothing parses, so parsing measures nothing. The case checks
// that the brief cites the thread the room turns on. It also checks that the
// brief names a word only that thread said, which the floor does not.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/roombrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type roomBriefCases struct{}

// The contract's spellings a fixture is folded into.
const (
	roomThreadOpen     = "open"
	roomThreadResolved = "resolved"
	roomSideSeller     = "seller"
)

func (roomBriefCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskSummarize, Variant: "deal_room_brief", Kind: ai.SiteKindOneShot}
}

type roomBriefFixture struct {
	Title     string              `json:"title"`
	State     string              `json:"state"`
	Buyers    []roomBriefBuyer    `json:"buyers"`
	Documents []roomBriefDocument `json:"documents"`
	Threads   []roomBriefThread   `json:"threads"`
}

type roomBriefBuyer struct {
	Label             string   `json:"label"`
	Name              string   `json:"name"`
	SignedIn          bool     `json:"signed_in"`
	LastSeenOn        string   `json:"last_seen_on"`
	AskedForNewLinkOn string   `json:"asked_for_new_link_on"`
	Downloaded        []string `json:"downloaded"`
}

type roomBriefDocument struct {
	Label string `json:"label"`
	Title string `json:"title"`
	Group string `json:"group"`
}

type roomBriefThread struct {
	Label         string                `json:"label"`
	DocumentLabel string                `json:"document_label"`
	Resolved      bool                  `json:"resolved"`
	Comments      []roombrief.CommentIn `json:"comments"`
}

type roomBriefExpectation struct {
	CitesLabel string   `json:"cites_label"`
	NamesToken string   `json:"names_token"`
	Avoids     []string `json:"avoids"`
}

//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (roomBriefCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f roomBriefFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("summarize/deal_room_brief: the fixture is not the shape this site takes: %w", err)
	}
	var want roomBriefExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("summarize/deal_room_brief: the expected answer is not this site's shape: %w", err)
	}
	in, byLabel, err := roomBriefInput(f)
	if err != nil {
		return nil, err
	}
	if err := refuseUnmeasurableRoomBrief(in, byLabel, want); err != nil {
		return nil, err
	}
	return &roomBriefCase{
		in: in, mustCite: byLabel[want.CitesLabel], citeLabel: want.CitesLabel,
		mustName: want.NamesToken, mustAvoid: want.Avoids,
	}, nil
}

// roomBriefInput mints an id per labelled record, builds the room's reads as
// the store returns them, and folds them through roombrief.FromRoom. So the
// waiting rule, the order and the caps are production's.
func roomBriefInput(f roomBriefFixture) (roombrief.Input, map[string]string, error) {
	byLabel := map[string]string{}
	mint := func(label string) (openapi_types.UUID, error) {
		if strings.TrimSpace(label) == "" || byLabel[label] != "" {
			return openapi_types.UUID{}, fmt.Errorf("summarize/deal_room_brief: the label %q is blank or repeated, so an expectation naming it means nothing", label)
		}
		id := ids.NewV7()
		byLabel[label] = id.String()
		return openapi_types.UUID(id), nil
	}
	var participants []crmcontracts.DealRoomParticipant
	for _, b := range f.Buyers {
		id, err := mint(b.Label)
		if err != nil {
			return roombrief.Input{}, nil, err
		}
		participants = append(participants, roomBriefParticipant(id, b))
	}
	var documents []crmcontracts.DealRoomDocument
	docIDs := map[string]openapi_types.UUID{}
	for _, d := range f.Documents {
		id, err := mint(d.Label)
		if err != nil {
			return roombrief.Input{}, nil, err
		}
		docIDs[d.Label] = id
		documents = append(documents, crmcontracts.DealRoomDocument{
			Id: id, Title: d.Title, GroupKey: crmcontracts.DealRoomDocumentGroup(d.Group),
		})
	}
	var threads []crmcontracts.DealRoomThread
	for _, t := range f.Threads {
		id, err := mint(t.Label)
		if err != nil {
			return roombrief.Input{}, nil, err
		}
		thread, err := roomBriefThreadRead(id, t, docIDs)
		if err != nil {
			return roombrief.Input{}, nil, err
		}
		threads = append(threads, thread)
	}
	room := crmcontracts.DealRoom{Title: f.Title, State: crmcontracts.DealRoomState(f.State)}
	return roombrief.FromRoom(room, participants, documents, threads), byLabel, nil
}

func roomBriefParticipant(id openapi_types.UUID, b roomBriefBuyer) crmcontracts.DealRoomParticipant {
	signedIn := b.SignedIn
	p := crmcontracts.DealRoomParticipant{Id: id, FullName: b.Name, HasSignedIn: &signedIn}
	if seen, err := time.Parse(time.DateOnly, b.LastSeenOn); err == nil {
		p.LastSeenAt = &seen
	}
	if asked, err := time.Parse(time.DateOnly, b.AskedForNewLinkOn); err == nil {
		// Late in the day, so a request on the day a buyer was last seen is
		// still outstanding.
		asked = asked.Add(23 * time.Hour)
		p.LinkRequestedAt = &asked
	}
	if len(b.Downloaded) > 0 {
		downloaded := b.Downloaded
		p.DocumentsDownloaded = &downloaded
	}
	return p
}

func roomBriefThreadRead(
	id openapi_types.UUID, t roomBriefThread, docIDs map[string]openapi_types.UUID,
) (crmcontracts.DealRoomThread, error) {
	if len(t.Comments) == 0 {
		return crmcontracts.DealRoomThread{}, fmt.Errorf("summarize/deal_room_brief: thread %q has no comments", t.Label)
	}
	thread := crmcontracts.DealRoomThread{Id: id, State: roomThreadOpen}
	if t.Resolved {
		thread.State = roomThreadResolved
	}
	if t.DocumentLabel != "" {
		doc, ok := docIDs[t.DocumentLabel]
		if !ok {
			return thread, fmt.Errorf("summarize/deal_room_brief: thread %q is about %q, which the fixture does not carry", t.Label, t.DocumentLabel)
		}
		thread.DocumentId = &doc
	}
	for _, c := range t.Comments {
		at, err := time.Parse(time.DateOnly, c.On)
		if err != nil {
			return thread, fmt.Errorf("summarize/deal_room_brief: a comment on thread %q has no date: %w", t.Label, err)
		}
		side := roomSideSeller
		if c.Speaker == roomSideBuyer {
			side = roomSideBuyer
		}
		thread.Comments = append(thread.Comments, crmcontracts.DealRoomComment{
			Author: crmcontracts.DealRoomAuthor{Name: c.Author, Side: side}, Body: c.Body, CreatedAt: at,
		})
	}
	thread.CreatedAt = thread.Comments[0].CreatedAt
	return thread, nil
}

// refuseUnmeasurableRoomBrief refuses a scenario no reply could meet, and one
// any reply meets: a token the floor already says passes without the model.
func refuseUnmeasurableRoomBrief(in roombrief.Input, byLabel map[string]string, want roomBriefExpectation) error {
	if byLabel[want.CitesLabel] == "" {
		return fmt.Errorf("summarize/deal_room_brief: the expectation cites %q, which the fixture does not carry", want.CitesLabel)
	}
	token := strings.ToLower(strings.TrimSpace(want.NamesToken))
	if token == "" {
		return errors.New("summarize/deal_room_brief: the expectation names no token, so a generic brief would pass")
	}
	summary, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("summarize/deal_room_brief: encode the summary: %w", err)
	}
	if !strings.Contains(strings.ToLower(string(summary)), token) {
		return fmt.Errorf("summarize/deal_room_brief: the token %q is in nothing the summary carries, so only an invented brief could name it", want.NamesToken)
	}
	if strings.Contains(strings.ToLower(claims.Prose(roombrief.Deterministic(in, "en"))), token) {
		return fmt.Errorf("summarize/deal_room_brief: the token %q is already in the floor's own prose, so a reply saying nothing would pass", want.NamesToken)
	}
	for _, avoided := range want.Avoids {
		if strings.TrimSpace(avoided) == "" {
			return errors.New("summarize/deal_room_brief: the expectation forbids a blank phrase, which every reply contains")
		}
	}
	return nil
}

type roomBriefCase struct {
	in        roombrief.Input
	mustCite  string
	citeLabel string
	mustName  string
	mustAvoid []string
}

// Run issues the production request, in English pinned so a record's score
// does not move with an installation's settings.
func (c *roomBriefCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := roombrief.BriefRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("summarize/deal_room_brief: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

func (c *roomBriefCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	sentences, err := roombrief.ParseBrief(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("the grounding filter refused the reply: %v", err)}
	}
	if len(sentences) == 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeAbstained, Detail: "no sentence cited a record of this room"}
	}
	var wrong []string
	if !cites(sentences, c.mustCite) {
		wrong = append(wrong, fmt.Sprintf("the brief never cites %q, the thread this room turns on", c.citeLabel))
	}
	folded := strings.ToLower(claims.Prose(sentences))
	if !strings.Contains(folded, strings.ToLower(c.mustName)) {
		wrong = append(wrong, fmt.Sprintf("the brief never names %q, so it would read the same about any room", c.mustName))
	}
	for _, avoided := range c.mustAvoid {
		if strings.Contains(folded, strings.ToLower(avoided)) {
			wrong = append(wrong, fmt.Sprintf("the brief says %q", avoided))
		}
	}
	if len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
