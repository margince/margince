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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// handledWindow is how far back a receipt is worth reporting when no window
// is bound: a day, the lane's own default.
const handledWindow = 24 * time.Hour

// HandledWindow answers where Done for you starts for this reader. Bound in
// compose to the done lane's own resolver, so both read one window.
type HandledWindow func(ctx context.Context, asOf time.Time) (time.Time, error)

// WithHandledWindow binds the window the receipts are read over.
func (s *Service) WithHandledWindow(w HandledWindow) *Service {
	s.handledWindow = w
	return s
}

// ReceiptActor is who did an act and on whose authority.
type ReceiptActor struct {
	Type       string
	ID         string
	OnBehalfOf *ids.UUID
}

// ReceiptDetails describes a page of receipts past their summary: who acted,
// named for a reader, and what the act moved. Optional: unbound, receipts
// carry neither, which is the contract an older server had.
type ReceiptDetails interface {
	Describe(ctx context.Context, receipts []Receipt) ([]ReceiptDetail, error)
}

// ReceiptDetail is one receipt's description, in the order it was asked for.
type ReceiptDetail struct {
	Actor *crmcontracts.ReceiptActor
	// Changes is nil when none of the act's audit rows could be read, which
	// is not an act that moved nothing.
	Changes *[]crmcontracts.MagicFieldChange
}

// handledCap bounds one page of receipts.
//
// A reader checking what was done needs to be able to finish reading. The list
// says when it stopped short rather than presenting the bound as the whole, and
// hands back where the next page starts.
const handledCap = 50

// WithReceiptDetails binds who acted and what each act moved. Unbound, a
// receipt carries its summary alone.
func (s *Service) WithReceiptDetails(d ReceiptDetails) *Service {
	s.receiptDetails = d
	return s
}

// HandledForYou is what was done, newest first, from the page `cursor` names.
func (s *Service) HandledForYou(ctx context.Context, cursor *string) (crmcontracts.HandledForYou, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.HandledForYou{}, err
	}
	position, err := decodeHandledCursor(cursor)
	if err != nil {
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
	if position.Since.IsZero() {
		if position.Since, err = s.handledSince(ctx, asOf); err != nil {
			return crmcontracts.HandledForYou{}, err
		}
	}
	since := position.Since
	// One MORE than the page carries, so the bound can be reported as reached
	// rather than inferred from a full page — a page that happened to hold
	// exactly the cap would otherwise claim there was more when there was not.
	recent, err := s.receipts.Recent(ctx, since, position.Offset+handledCap+1)
	if err != nil {
		return crmcontracts.HandledForYou{}, err
	}
	recent = position.pin(recent)
	out := crmcontracts.HandledForYou{
		AsOf:      asOf,
		Since:     &since,
		Receipts:  make([]crmcontracts.Receipt, 0, handledCap),
		Truncated: len(recent) > position.Offset+handledCap,
	}
	page := recent[min(position.Offset, len(recent)):min(position.Offset+handledCap, len(recent))]
	if out.Truncated && position.Offset+len(page) <= maxHandledOffset {
		next, err := position.after(len(page), recent)
		if err != nil {
			return crmcontracts.HandledForYou{}, err
		}
		out.NextCursor = &next
	}
	if err := s.drawReceipts(ctx, page, &out); err != nil {
		return crmcontracts.HandledForYou{}, err
	}
	return out, nil
}

// handledSince is where the window starts: the done lane's own resolver where
// one is bound, a day otherwise.
func (s *Service) handledSince(ctx context.Context, asOf time.Time) (time.Time, error) {
	if s.handledWindow == nil {
		return asOf.Add(-handledWindow), nil
	}
	return s.handledWindow(ctx, asOf)
}

// drawReceipts renders one page, then names its records and sources under the
// reader's own grants.
func (s *Service) drawReceipts(ctx context.Context, page []Receipt, out *crmcontracts.HandledForYou) error {
	var details []ReceiptDetail
	if s.receiptDetails != nil && len(page) > 0 {
		var err error
		if details, err = s.receiptDetails.Describe(ctx, page); err != nil {
			return err
		}
	}
	subjects := make([]*crmcontracts.AttentionSubject, 0, 2*len(page))
	for i, receipt := range page {
		row := handledReceipt(receipt)
		if i < len(details) {
			row.Actor, row.Changes = details[i].Actor, details[i].Changes
		}
		out.Receipts = append(out.Receipts, row)
		subjects = append(subjects, row.Subject, row.SourceRecord)
	}
	// The same label pass the feed runs, so a receipt names its record here
	// exactly as it does in the feed's done lane.
	if err := s.labelSubjects(ctx, subjects); err != nil {
		return err
	}
	// A source the reader may not open is left out rather than named. The record
	// the act was about is the receipt's claim; its source is not.
	for i := range out.Receipts {
		if source := out.Receipts[i].SourceRecord; source != nil && source.Label == nil {
			out.Receipts[i].SourceRecord = nil
		}
	}
	return nil
}

// handledReceipt is one completed act on the wire.
func handledReceipt(receipt Receipt) crmcontracts.Receipt {
	out := crmcontracts.Receipt{
		Id:              openapi_types.UUID(receipt.ID),
		Review:          receipt.Review,
		CloseDateChange: receipt.CloseDateChange,
		Kind:            receipt.Kind,
		Summary:         receipt.Summary,
		OccurredAt:      receipt.OccurredAt,
	}
	// The record it was about, where the act named one. Not every approval is
	// about a record, and an absent subject is a real state — the client draws
	// the summary alone rather than inventing something to open.
	if subject := subjectOf(receipt.TargetType, receipt.TargetID); subject != nil {
		out.Subject = subject
	}
	if !receipt.SourceActivityID.IsZero() {
		out.SourceRecord = subjectOf("activity", receipt.SourceActivityID)
	}
	// The way back, only where the act carries one. An approval the system
	// decided is revisitable through the record it named; a correction nobody
	// was asked about has this row as its only telling, so the undo travels on
	// it or it is not offered at all.
	if undo := receipt.Undo; undo != nil {
		out.Undo = &crmcontracts.AppliedUndo{
			AuditLogId: openapi_types.UUID(undo.AuditLogID),
			Version:    undo.Version,
			Reversed:   undo.Reversed,
		}
	}
	return out
}
