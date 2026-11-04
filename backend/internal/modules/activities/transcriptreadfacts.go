// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a finished transcript reading did, kept as facts rather than as a
// sentence. The report is phrased from them when it is read, so each reader
// gets their own language, "you" for their own work and a name for anyone
// else's. status_detail stays beside them for the readings written before.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The failure codes a reading can close with.
const (
	TranscriptFailureNotTranscript = "not_transcript"
	TranscriptFailureTooLong       = "too_long"
	TranscriptFailureBlank         = "blank"
	TranscriptFailureMissing       = "missing"
	TranscriptFailureRefused       = "refused"
	TranscriptFailureNoModel       = "no_model"
)

// TranscriptReadFacts is what one reading did with what it found.
type TranscriptReadFacts struct {
	// Found is how many promises the reading kept, before the rule sorted
	// them. A reading that staged nothing says "stated nothing" at zero and
	// "already put to you" above it.
	Found     int                     `json:"found"`
	Tasks     []TranscriptSeatCount   `json:"tasks,omitempty"`
	Watched   int                     `json:"watched,omitempty"`
	Proposals []TranscriptProposal    `json:"proposals,omitempty"`
	Withheld  []TranscriptReasonCount `json:"withheld,omitempty"`
	Failure   *TranscriptFailure      `json:"failure,omitempty"`
}

// TranscriptSeatCount is how many tasks went to one seat.
type TranscriptSeatCount struct {
	SeatID ids.UUID `json:"seat_id"`
	Count  int      `json:"count"`
}

// TranscriptProposal is one staged approval and the seat it waits on.
type TranscriptProposal struct {
	ApprovalID ids.UUID `json:"approval_id"`
	SeatID     ids.UUID `json:"seat_id"`
}

// TranscriptReasonCount is how many promises one reason withheld.
type TranscriptReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// TranscriptFailure is why a reading could not be used. Lines and Chars are
// the transcript's size, set only for too_long.
type TranscriptFailure struct {
	Code  string `json:"code"`
	Lines int    `json:"lines,omitempty"`
	Chars int    `json:"chars,omitempty"`
}

// encodeTranscriptFacts is the column value: NULL for an outcome that states
// no facts.
func encodeTranscriptFacts(facts *TranscriptReadFacts) ([]byte, error) {
	if facts == nil {
		return nil, nil
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("activities: encoding a transcript reading's outcome: %w", err)
	}
	return raw, nil
}

// decodeTranscriptFacts reads the column back. NULL is a reading from before
// facts were kept, or one still running.
func decodeTranscriptFacts(raw []byte) (*TranscriptReadFacts, error) {
	if raw == nil {
		return nil, nil
	}
	var facts TranscriptReadFacts
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, fmt.Errorf("activities: decoding a transcript reading's outcome: %w", err)
	}
	return &facts, nil
}
