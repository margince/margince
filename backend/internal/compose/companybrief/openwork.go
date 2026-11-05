// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// The promises read out of conversations, as the brief and the answers read
// them. They are not tasks: nobody filed them, and nothing on file names an
// owner on our side.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Sides a commitment can be on, as the contract spells them.
const (
	sideOurs   = string(crmcontracts.Company360CommitmentKindOurs)
	sideTheirs = string(crmcontracts.Company360CommitmentKindTheirs)
)

// CommitmentIn is one open promise read out of a conversation.
//
// ID is the claim's own, since one message can hold two promises, and
// ActivityID is that message, which is what a citation opens.
type CommitmentIn struct {
	ID         string `json:"id"`
	ActivityID string `json:"activity_id"`
	Claim      string `json:"claim"`
	// Side says whose promise it is: "ours" or "theirs".
	Side string `json:"side"`
	// Holder is the contact who made a promise to us. It is empty for one we
	// made, because nothing on file says who owns it and none is guessed.
	Holder string `json:"holder,omitempty"`
	// Due is RFC3339 in UTC, empty when the promise carries no date.
	Due string `json:"due,omitempty"`
}

func foldCommitments(view crmcontracts.Company360, in *Input) {
	if view.OpenCommitments == nil {
		return
	}
	for _, claim := range view.OpenCommitments.Data {
		open := CommitmentIn{
			ID: claim.ClaimId.String(), ActivityID: claim.ActivityId.String(),
			Claim: claim.Body, Side: string(claim.Kind),
		}
		if claim.Kind == crmcontracts.Company360CommitmentKindTheirs {
			open.Holder = claim.ContactName
		}
		if claim.DueAt != nil {
			open.Due = claim.DueAt.UTC().Format(time.RFC3339)
		}
		in.OpenCommitments = append(in.OpenCommitments, open)
	}
}

// foldOpenWorkGaps records that a list of open work is shorter than the open
// work. Either a page was cut or the reader's row scope narrowed the read.
func foldOpenWorkGaps(view crmcontracts.Company360, in *Input) {
	in.OpenWorkCapped = (view.Deals != nil && view.Deals.Page.HasMore) ||
		(view.NextSteps != nil && view.NextSteps.Page.HasMore) ||
		(view.OpenCommitments != nil && view.OpenCommitments.Page.HasMore)
	in.OpenWorkWithheld = view.AttentionWithheld != nil && *view.AttentionWithheld
}
