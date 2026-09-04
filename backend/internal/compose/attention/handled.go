// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the product did on this reader's behalf.
//
// The queue lists obligations; this is the opposite surface, and the only place
// a reader can check what was done without being asked to do anything about it.
// A product that acts autonomously and never says what it acted on is asking
// for trust it has not earned.
//
// FROM THE SAME SEAM the day's receipt lane already reads. A second path to one
// answer would drift the first time either changed, and the two would then
// disagree about what the product did — which is the one thing this surface
// exists to be reliable about.
//
// NEVER AN OBLIGATION. `done_for_you` is deliberately absent from the worklist's
// own source vocabulary, so nothing here can come back as a row. A receipt that
// reappeared as work would ask the reader to redo what was already done.

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// handledWindow is how far back a receipt is worth reporting.
//
// A day, matching the lane the feed already draws. Past that a reader is
// auditing rather than checking, and an audit is the trail's job — this surface
// answers "what happened while I was away", which is a question about today.
const handledWindow = 24 * time.Hour

// handledCap bounds one page of receipts.
//
// A reader checking what was done needs to be able to finish reading. The list
// says when it stopped short rather than presenting the bound as the whole.
const handledCap = 50

// HandledForYou is what was done, newest first.
func (s *Service) HandledForYou(ctx context.Context) (crmcontracts.HandledForYou, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.HandledForYou{}, err
	}
	if s.receipts == nil {
		// UNBOUND is an empty page, not a refusal, and the difference matters
		// here. An installation that wires no receipt reader did nothing on
		// anybody's behalf to report — which is honestly nothing to show,
		// rather than a surface the reader may not see.
		return crmcontracts.HandledForYou{
			AsOf:     s.now(),
			Receipts: []crmcontracts.Receipt{},
		}, nil
	}
	asOf := s.now()
	// One MORE than the page carries, so the bound can be reported as reached
	// rather than inferred from a full page — a page that happened to hold
	// exactly the cap would otherwise claim there was more when there was not.
	recent, err := s.receipts.Recent(ctx, asOf.Add(-handledWindow), handledCap+1)
	if err != nil {
		return crmcontracts.HandledForYou{}, err
	}
	out := crmcontracts.HandledForYou{
		AsOf:      asOf,
		Receipts:  make([]crmcontracts.Receipt, 0, len(recent)),
		Truncated: len(recent) > handledCap,
	}
	if out.Truncated {
		recent = recent[:handledCap]
	}
	for _, receipt := range recent {
		out.Receipts = append(out.Receipts, handledReceipt(receipt))
	}
	return out, nil
}

// handledReceipt is one completed act on the wire.
func handledReceipt(receipt Receipt) crmcontracts.Receipt {
	out := crmcontracts.Receipt{
		Id:         openapi_types.UUID(receipt.ID),
		Kind:       receipt.Kind,
		Summary:    receipt.Summary,
		OccurredAt: receipt.OccurredAt,
	}
	// The record it was about, where the act named one. Not every approval is
	// about a record, and an absent subject is a real state — the client draws
	// the summary alone rather than inventing something to open.
	if subject := subjectOf(receipt.TargetType, receipt.TargetID); subject != nil {
		out.Subject = subject
	}
	return out
}
