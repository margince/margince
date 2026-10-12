// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The one verb that settles a pair the review queue holds: dismiss it as two
// different records, or take the dismissal back.
//
// A dismissal suppresses the pair from every later sweep, which is why it is a
// human's call on the web screen. It is an agent's here because it can be
// undone: "reopen" lifts the suppression and the pair is back in everyone's
// queue. It is one tool with a decision, as decide_approval is, because each
// tool spends one of the client's slots on every listing. Merging is not a
// decision here: one merge in the system is merge_records.

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// The two decisions decide_duplicate takes.
const (
	// DuplicateNotTheSame dismisses the pair as two different records.
	DuplicateNotTheSame = "not_the_same"
	// DuplicateReopen lifts a dismissal.
	DuplicateReopen = "reopen"
)

// DuplicateVerdict is where a pair stands after a decision settled it.
type DuplicateVerdict struct {
	CandidateID string `json:"candidate_id"`
	// Disposition is "not_a_duplicate" after a dismissal and "open" after a
	// re-open.
	Disposition string `json:"disposition"`
}

// DuplicateQueue is the seam onto the contacts module's review queue.
type DuplicateQueue interface {
	// Dismiss records that the two records in the pair are different.
	Dismiss(ctx context.Context, candidate ids.UUID) (DuplicateVerdict, error)
	// Reopen lifts a dismissal. A merged pair is refused: a merge is not undone
	// from here.
	Reopen(ctx context.Context, candidate ids.UUID) (DuplicateVerdict, error)
}

// RegisterDuplicateTools registers the queue verb, or nothing where no queue is
// bound.
func RegisterDuplicateTools(r *Registry, queue DuplicateQueue) {
	if queue == nil {
		return
	}
	r.Register(decideDuplicate{queue: queue})
}

type decideDuplicate struct{ queue DuplicateQueue }

func (t decideDuplicate) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "decide_duplicate", Title: "Decide a flagged duplicate pair", Version: toolVersionV1,
		Description:   decideDuplicateCopy.render(),
		Instead:       decideDuplicateCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "disposeDedupeCandidate/undoDedupeDisposition",
		InputSchema: schema(`{"type":"object","required":["candidate_id","decision"],"properties":{
			"candidate_id":{"type":"string","format":"uuid","description":"The pair's candidate_id, from the duplicate_candidates a create answered with"},
			"decision":{"type":"string","enum":["` + DuplicateNotTheSame + `","` + DuplicateReopen + `"],"description":"not_the_same dismisses the pair; reopen takes a dismissal back"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[DuplicateVerdict](),
	}
}

func (t decideDuplicate) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		CandidateID ids.UUID `json:"candidate_id"`
		Decision    string   `json:"decision"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	var (
		verdict DuplicateVerdict
		err     error
	)
	switch args.Decision {
	case DuplicateNotTheSame:
		verdict, err = t.queue.Dismiss(ctx, args.CandidateID)
	case DuplicateReopen:
		verdict, err = t.queue.Reopen(ctx, args.CandidateID)
	default:
		return nil, &BadArgsError{Cause: errors.New("decision must be not_the_same or reopen")}
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(verdict)
}
