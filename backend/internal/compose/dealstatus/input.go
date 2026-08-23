// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// What one card is written from, and the fingerprint that decides whether a
// cached one still stands.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// promptVersion changes whenever the prompt or the projection changes, so a
// card written by the old wording is rewritten rather than served forever. It
// is part of the fingerprint for exactly that reason.
const promptVersion = "deal-status-2"

// project renders the gathered facts into the prompt's shape. Only what the
// caller may read reaches it: the facts were gathered under their row scope,
// and a withheld row gives up its words here.
func project(f facts, move crmcontracts.DealStatusCardMove) StatusInput {
	in := StatusInput{Deal: dealIn(f.deal), RecommendedMove: move.Action + ": " + move.Reason}
	in.Health = healthIn(f.health)
	for _, a := range f.timeline {
		if len(in.Timeline) == maxTimelineRows {
			break
		}
		in.Timeline = append(in.Timeline, actIn(a, f.now))
	}
	for _, t := range f.openTasks {
		in.OpenTasks = append(in.OpenTasks, taskIn(t))
	}
	in.Room = roomIn(f)
	return in
}

// healthIn carries the four factors as measurements: a score and, where the
// factor counts things, the count. No sentence — see FactorIn for why.
func healthIn(h *deals.DealHealth) []FactorIn {
	if h == nil {
		return nil
	}
	engaged := len(h.Evidence.EngagedStakeholderIDs)
	overdue := len(h.Evidence.OverdueTaskIDs)
	return []FactorIn{
		{Key: "activity_recency", Value: h.Factors.ActivityRecency},
		{Key: "stage_velocity", Value: h.Factors.StageVelocity},
		{Key: "engagement", Value: h.Factors.Engagement, Count: &engaged},
		{Key: "commitments", Value: h.Factors.Commitments, Count: &overdue},
	}
}

func actIn(a crmcontracts.Activity, now time.Time) ActIn {
	out := ActIn{
		ID: a.Id.String(), Kind: string(a.Kind),
		At: a.OccurredAt.Format("2006-01-02"), When: whenOf(a, now),
	}
	if a.Direction != nil {
		out.Direction = string(*a.Direction)
	}
	// A withheld row keeps its place in the story — contact happened on this
	// date — and gives up its words: the reader may not open them, so the
	// model may not read them.
	if withheld(a) {
		return out
	}
	if a.Subject != nil {
		out.Subject = *a.Subject
	}
	if a.Body != nil {
		out.Excerpt = excerpt(*a.Body)
	}
	return out
}

// whenOf says whether a row has happened. The deal's timeline holds booked
// meetings alongside past contact, and only this tells them apart.
func whenOf(a crmcontracts.Activity, now time.Time) string {
	if a.OccurredAt.After(now) {
		return "scheduled"
	}
	return "past"
}

func withheld(a crmcontracts.Activity) bool {
	return a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld
}

func taskIn(t activities.OpenTask) TaskIn {
	out := TaskIn{ID: t.ID.String(), Subject: t.Subject}
	if t.DueAt != nil {
		out.Due = t.DueAt.Format("2006-01-02")
	}
	return out
}

// roomIn carries the room's state and its conversation. A required-change
// thread still open is the clearest risk signal a room holds, so it is named
// rather than left for the model to infer from an opener's wording.
func roomIn(f facts) *RoomIn {
	if f.room == nil {
		return nil
	}
	out := &RoomIn{State: string(f.room.State)}
	for _, th := range f.threads {
		if len(out.Threads) == maxThreadRows {
			break
		}
		out.Threads = append(out.Threads, threadIn(th))
	}
	return out
}

func threadIn(th crmcontracts.DealRoomThread) ThreadIn {
	out := ThreadIn{ID: th.Id.String(), State: string(th.State)}
	out.RequiredChange = th.RequiredChange
	if len(th.Comments) > 0 {
		out.Opener = excerpt(th.Comments[0].Body)
	}
	return out
}

// Fingerprint keys the cache on everything that could change the card: the
// assembled input, the prompt version, the reader, and the DAY.
//
// The day is in the key because the card says "the last contact was 4 days
// ago" — prose rendered from the clock at write time, over facts that have not
// moved. Without it a card written on Monday still says "4 days ago" on
// Friday, and the fingerprint has no reason to notice. A day's granularity is
// exactly what those sentences resolve to; anything finer would rewrite a
// quiet deal for nothing.
//
// The READER is in the key because the input was assembled under their row
// scope. It is a belt over the braces rather than the isolation itself: what
// keeps two readers apart is the (user_id, deal_id) primary key and the
// explicit user_id predicate in Service.cached, where user_id comes from the
// authenticated principal and never from the request.
func Fingerprint(in StatusInput, userID ids.UUID, routingVersion string, day time.Time) (string, error) {
	encoded, err := json.Marshal(struct {
		In      StatusInput `json:"in"`
		Prompt  string      `json:"prompt"`
		Routing string      `json:"routing"`
		Reader  string      `json:"reader"`
		Day     string      `json:"day"`
	}{
		In: in, Prompt: promptVersion, Routing: routingVersion,
		Reader: userID.String(), Day: day.UTC().Format("2006-01-02"),
	})
	if err != nil {
		return "", fmt.Errorf("fingerprint the deal status input: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
