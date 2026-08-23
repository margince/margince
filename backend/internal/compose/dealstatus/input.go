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
const promptVersion = "deal-status-1"

// project renders the gathered facts into the prompt's shape. Only what the
// caller may read reaches it: the facts were gathered under their row scope,
// and a withheld row gives up its words here.
func project(f facts, move crmcontracts.DealStatusCardMove) StatusInput {
	in := StatusInput{Deal: dealIn(f.deal), RecommendedMove: move.Action + ": " + move.Reason}
	in.Health = healthIn(f.health, f.now)
	for _, a := range f.timeline {
		if len(in.Timeline) == maxTimelineRows {
			break
		}
		in.Timeline = append(in.Timeline, actIn(a))
	}
	for _, t := range f.openTasks {
		in.OpenTasks = append(in.OpenTasks, taskIn(t))
	}
	in.Room = roomIn(f)
	return in
}

// healthIn carries the four factors with the fact behind each, so the model
// can say WHY a deal is at risk rather than that a number is low. A reader
// cannot act on 0.31.
//
// The wording is the deals module's, not this package's: the formula and the
// sentences that explain it belong together.
func healthIn(h *deals.DealHealth, now time.Time) []FactorIn {
	if h == nil {
		return nil
	}
	reasons := deals.HealthReasons(*h, now)
	out := make([]FactorIn, 0, len(reasons))
	for _, r := range reasons {
		out = append(out, FactorIn{Key: r.Key, Value: r.Value, Reason: r.Reason})
	}
	return out
}

func actIn(a crmcontracts.Activity) ActIn {
	out := ActIn{ID: a.Id.String(), Kind: string(a.Kind), At: a.OccurredAt.Format("2006-01-02")}
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
// assembled input, the prompt version, and the reader.
//
// The READER is in the key because the input was assembled under their row
// scope. Two people with different grants see different facts, so a card
// written for one is not a card the other may read — sharing the cache row
// would either leak a scoped activity or serve the deal's owner the
// restricted colleague's thinner card.
func Fingerprint(in StatusInput, userID ids.UUID, routingVersion string) (string, error) {
	encoded, err := json.Marshal(struct {
		In      StatusInput `json:"in"`
		Prompt  string      `json:"prompt"`
		Routing string      `json:"routing"`
		Reader  string      `json:"reader"`
	}{In: in, Prompt: promptVersion, Routing: routingVersion, Reader: userID.String()})
	if err != nil {
		return "", fmt.Errorf("fingerprint the deal status input: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
