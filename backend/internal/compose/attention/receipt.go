// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Receipt is one completed autonomous act, reported rather than asked about.
type Receipt struct {
	SystemDisplay *crmcontracts.SystemDisplay
	// Source says which identity ID is; AuditLogID is the correction's audit
	// entry, kept apart from Undo because a receipt without Undo still has one.
	Source          ReceiptSource
	AuditLogID      ids.UUID
	CloseDateChange *crmcontracts.CloseDateChange
	Review          *crmcontracts.AppliedDealChangeReview
	ID              ids.UUID
	Kind            string
	Summary         string
	SummaryParts    *crmcontracts.ApprovalSummaryParts
	OccurredAt      time.Time
	// The record the decision was about, carried from the approval it came
	// from. A reader told the system sent something on their behalf wants the
	// account it went to, and the card offers `open` only when this names one.
	//
	// Empty TargetType means the decision named no record, which is a real
	// state: not every approval is about one.
	TargetType string
	TargetID   ids.UUID
	// TargetLabel is the target's name as the decision was staged.
	TargetLabel *string
	// Undo names the audit row the record-history restore route puts back, on
	// a receipt for work that was APPLIED rather than approved.
	//
	// Absent means there is nothing to offer. An approval the system decided
	// is revisited through its record, and a reversed correction has no second
	// undo in it.
	Undo *ReceiptUndo
	// Actor is who did it, as the audit trail spells them. A zero Type means
	// the source could not say.
	Actor ReceiptActor
	// SourceActivityID is the message the act was read from; zero when it
	// cites none.
	SourceActivityID ids.UUID
	// Headline is Summary without its appended reason and quote. A client that
	// draws Reason on its own line then prints it once.
	Headline string
	// Reason is why the act was taken, as recorded when it was; empty when
	// nothing was recorded or this reader may not read it.
	Reason string
	// CustomerQuote is the customer's own words the act rests on. It is kept
	// apart from Reason, which is Margince's reading and never a quote.
	CustomerQuote string
	// ReasonKind says whether Reason is a rule or an inference; empty when
	// the act cannot say.
	ReasonKind string
	// CitedActivities are the messages a change rests on. The newest the
	// reader may open becomes the receipt's source.
	CitedActivities []ids.UUID
}

// ReceiptUndo is what a receipt needs to offer a way back.
type ReceiptUndo struct {
	AuditLogID ids.UUID
	// Version of the record the restore route compares against, so two contacts
	// undoing the same change do not overwrite each other silently.
	Version int64
	// Reversed says somebody already put this back. The row stays and says so
	// rather than vanishing, which would leave a reader unsure their Undo
	// landed.
	Reversed bool
}

// ReceiptSource names the producer whose identity a receipt's ID is.
type ReceiptSource string

// The two producers: a deal correction Margince applied, and an approval a
// reader decided.
const (
	ReceiptFromCorrection ReceiptSource = "correction"
	ReceiptFromApproval   ReceiptSource = "approval"
)
