// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"errors"
	"regexp"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// actionRecordLimit keeps the stored references small; Count still totals them all.
const actionRecordLimit = 100

var actionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ActionNeeded is work a step finished but left for someone. The step still
// ends done, so later steps run, and System health shows the reason as a to-do.
type ActionNeeded struct {
	Reason  string      `json:"reason"`
	Count   int64       `json:"count"`
	Records []RecordRef `json:"records"`
}

// RecordRef names one record someone must act on, never its content.
type RecordRef struct {
	Type string   `json:"type"`
	ID   ids.UUID `json:"id"`
}

// mergeAction adds one batch's records to the step's stored action. A step has
// one reason, so a second reason is a registration fault, not a new to-do. A
// record already held is not counted again; a step may meet one record in two batches.
func mergeAction(stored, batch *ActionNeeded) (*ActionNeeded, error) {
	if batch == nil || len(batch.Records) == 0 {
		return stored, nil
	}
	if !actionCodePattern.MatchString(batch.Reason) {
		return nil, errors.New("data task action needs a reason code")
	}
	merged := ActionNeeded{Reason: batch.Reason}
	if stored != nil {
		if stored.Reason != batch.Reason {
			return nil, errors.New("data task step reported a second action reason")
		}
		merged.Count, merged.Records = stored.Count, append(merged.Records, stored.Records...)
	}
	for _, record := range batch.Records {
		if !actionCodePattern.MatchString(record.Type) || record.ID.IsZero() {
			return nil, errors.New("data task action record needs a type and an ID")
		}
		if slices.Contains(merged.Records, record) {
			continue
		}
		merged.Count++
		if len(merged.Records) < actionRecordLimit {
			merged.Records = append(merged.Records, record)
		}
	}
	return &merged, nil
}
