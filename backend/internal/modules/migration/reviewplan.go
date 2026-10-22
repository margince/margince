// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"encoding/json"
	"time"
)

// ReviewRowLimit bounds the complete set a human can authorize in one import review.
const ReviewRowLimit = 500

// ReviewRow retains checked values and source identity so release can compare the same import row.
type ReviewRow struct {
	Object          string                     `json:"object"`
	ExternalID      string                     `json:"external_id"`
	Line            int                        `json:"line"`
	Outcome         string                     `json:"outcome"`
	Values          map[string]json.RawMessage `json:"values"`
	CheckedValues   map[string]json.RawMessage `json:"checked_values"`
	Links           []ReviewLink               `json:"links"`
	TargetID        string                     `json:"target_id,omitempty"`
	TargetVersion   int64                      `json:"target_version,omitempty"`
	OwnerExternalID string                     `json:"owner_external_id,omitempty"`
	LastSyncedAt    time.Time                  `json:"last_synced_at"`
}

// ReviewLink distinguishes resolved references from links waiting on another imported row.
type ReviewLink struct {
	Object     string `json:"object"`
	Key        string `json:"key"`
	TargetID   string `json:"target_id,omitempty"`
	Pending    bool   `json:"pending"`
	PendingKey string `json:"pending_key,omitempty"`
	Dropped    bool   `json:"dropped"`
	ByName     bool   `json:"by_name"`
}

// ReviewPlan binds an import decision set to its actor and completeness verdict.
type ReviewPlan struct {
	ActorID      string      `json:"actor_id"`
	Version      int         `json:"version"`
	Complete     bool        `json:"complete"`
	Enforced     bool        `json:"enforced,omitempty"`
	Rows         []ReviewRow `json:"rows"`
	Associations []Assoc     `json:"associations"`
}

// Add invalidates the whole review when its row set is too large or ambiguous.
func (p *ReviewPlan) Add(row ReviewRow) {
	if !p.Complete {
		return
	}
	if len(p.Rows) >= ReviewRowLimit {
		p.Complete = false
		p.Rows = nil
		return
	}
	for _, prior := range p.Rows {
		if prior.Object == row.Object && (prior.ExternalID == row.ExternalID || row.TargetID != "" && prior.TargetID == row.TargetID) {
			p.Complete, p.Rows = false, nil
			return
		}
	}
	p.Rows = append(p.Rows, row)
}
