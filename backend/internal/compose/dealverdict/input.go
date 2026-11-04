// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
)

// The kinds of change the model may propose.
const (
	KindSetCloseDate   = "set_close_date"
	KindSetAmount      = "set_amount"
	KindSetProbability = "set_probability"
)

// Input is what one deal's verdict reads. Activities are only those every
// member may read: the reason and its citations reach Home and the deal.
type Input struct {
	Today             time.Time
	DealName          string
	Stage             string
	AmountMinor       *int64
	Currency          *string
	CloseDate         *time.Time
	Provisional       bool
	Probability       int
	ProbabilitySource string
	Offers            []Offer
	StageMoves        []StageMove
	Prior             []PriorAction
	Activities        []Activity
}

// Activity is one logged mail, meeting, call or note, newest last.
type Activity struct {
	ID         string
	Kind       string
	Direction  string
	OccurredAt time.Time
	Subject    string
	Body       string
}

// Offer is one offer on the deal.
type Offer struct {
	GrossMinor int64
	Currency   string
	Status     string
	Accepted   bool
}

// StageMove is one move in the deal's stage history.
type StageMove struct {
	From string
	To   string
	At   time.Time
}

// PriorAction is one action of the deal's previous verdict and what became
// of it.
type PriorAction struct {
	Kind    string `json:"kind"`
	Outcome string `json:"outcome"`
}

// State is what the rules in code check besides the model's answer: the
// deal's values as read, the owner's switches, the Undo holds and the last
// time a colleague edited each field.
type State struct {
	Seen deals.VerdictValues
	// SwitchedOn holds the correction names whose owner switch is on.
	SwitchedOn map[string]bool
	// Held holds the correction names an Undo still holds back.
	Held map[string]bool
	// HumanEditedAt is when a colleague last wrote each correction's field.
	HumanEditedAt map[string]time.Time
	// AcceptedOffer is the newest accepted offer, which fixes the amount.
	AcceptedOffer *Offer
}

// activityByID indexes the activities a citation may name.
func (in Input) activityByID() map[string]Activity {
	out := make(map[string]Activity, len(in.Activities))
	for _, activity := range in.Activities {
		out[activity.ID] = activity
	}
	return out
}
