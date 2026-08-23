// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// What a consumer of the Deal Room's events reads, and how it reads it.
//
// Exported from the module that EMITS them so a consumer never restates their
// shape: a second copy of the field names in another package keeps compiling
// after this one is renamed, and the consumer would then act on zero values
// rather than fail. The same reasoning deals/stagechanged.go states.

import (
	"encoding/json"
	"fmt"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The event types a Deal Room publishes that a reader of the deal's timeline
// cares about: what the two sides said, what a buyer decided, and what the
// seller released.
const (
	EventCommentPosted    = "deal_room.comment_posted"
	EventDecisionRecorded = "deal_room.decision_recorded"
	EventPublished        = "deal_room.published"
)

// CommentPosted is one comment as its consumers need it.
type CommentPosted struct {
	DealID      ids.UUID
	ThreadID    ids.UUID
	CommentID   ids.UUID
	DocumentID  *ids.UUID
	Side        string
	OpensThread bool
}

// DecodeCommentPosted reads one comment payload off the bus.
func DecodeCommentPosted(payload json.RawMessage) (CommentPosted, error) {
	var wire crmcontracts.PublicEventDealRoomCommentPosted
	if err := json.Unmarshal(payload, &wire); err != nil {
		return CommentPosted{}, fmt.Errorf("decode %s: %w", EventCommentPosted, err)
	}
	out := CommentPosted{
		DealID:      ids.UUID(wire.DealId),
		ThreadID:    ids.UUID(wire.ThreadId),
		CommentID:   ids.UUID(wire.CommentId),
		Side:        wire.Side,
		OpensThread: wire.OpensThread,
	}
	if wire.DocumentId != nil {
		doc := ids.UUID(*wire.DocumentId)
		out.DocumentID = &doc
	}
	return out, nil
}

// DecisionRecorded is one buyer decision on a document version.
type DecisionRecorded struct {
	DealID     ids.UUID
	DecisionID ids.UUID
	DocumentID ids.UUID
	Kind       string
}

// DecodeDecisionRecorded reads one decision payload off the bus.
func DecodeDecisionRecorded(payload json.RawMessage) (DecisionRecorded, error) {
	var wire crmcontracts.PublicEventDealRoomDecisionRecorded
	if err := json.Unmarshal(payload, &wire); err != nil {
		return DecisionRecorded{}, fmt.Errorf("decode %s: %w", EventDecisionRecorded, err)
	}
	return DecisionRecorded{
		DealID:     ids.UUID(wire.DealId),
		DecisionID: ids.UUID(wire.DecisionId),
		DocumentID: ids.UUID(wire.DocumentId),
		Kind:       wire.Kind,
	}, nil
}

// Published is one release of a room to its buyers.
type Published struct {
	DealID    ids.UUID
	ReleaseNo int
	ReleaseID *ids.UUID
}

// DecodePublished reads one publish payload off the bus.
func DecodePublished(payload json.RawMessage) (Published, error) {
	var wire crmcontracts.PublicEventDealRoomPublished
	if err := json.Unmarshal(payload, &wire); err != nil {
		return Published{}, fmt.Errorf("decode %s: %w", EventPublished, err)
	}
	out := Published{DealID: ids.UUID(wire.DealId), ReleaseNo: wire.ReleaseNo}
	if wire.ReleaseId != nil {
		rel := ids.UUID(*wire.ReleaseId)
		out.ReleaseID = &rel
	}
	return out, nil
}
