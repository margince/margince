// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam that lets a create tell its caller what it filed for review.
//
// The tool surface may not import the people module (.go-arch-lint.yml), and
// should not: an injected reader is what keeps `agents` unable to reach a
// record table on its own, so RBAC and row scope apply to this read exactly as
// they do on the HTTP path. This is the wiring, and it is thin on purpose —
// the store method it calls owns the permission check and the both-sides
// visibility rule, restated nowhere.

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// openDuplicatesFor reports the open review-queue pairs naming a record the
// caller has just created.
//
// The OTHER side is what the answer names. A caller holding a left/right pair
// has to work out which half is its own record before it can offer a merge, and
// that is a step the seam can take once instead of every caller taking it.
func openDuplicatesFor(pool *pgxpool.Pool) agents.OpenDuplicatesFor {
	store := people.NewStore(InstallationDB(pool))
	return func(ctx context.Context, recordType string, id ids.UUID) ([]agents.DuplicateCandidate, error) {
		rows, err := store.OpenCandidatesNaming(ctx, recordType, id)
		if err != nil {
			return nil, err
		}
		out := make([]agents.DuplicateCandidate, 0, len(rows))
		for _, r := range rows {
			other := r.RightID
			if other == id {
				other = r.LeftID
			}
			out = append(out, agents.DuplicateCandidate{
				OtherRecordID: other.String(),
				Confidence:    r.Confidence,
				Evidence:      decodeEvidence(r.Evidence),
			})
		}
		return out, nil
	}
}

// decodeEvidence renders the stored snapshot for the wire.
//
// A snapshot that will not decode yields NO evidence rather than failing the
// answer: the pair itself is the finding, and losing "a human was asked" over a
// malformed detail row would trade the whole message for part of it. The stored
// bytes are written by this system in a fixed shape, so this is a guard against
// the impossible rather than an expected branch.
func decodeEvidence(raw json.RawMessage) []agents.DuplicateEvidence {
	if len(raw) == 0 {
		return nil
	}
	// Through an intermediate whose values are `any`, because the stored
	// snapshot's left/right are whatever the matching field held — a phone
	// number is a string, a score is a number — and decoding straight into a
	// typed string member would fail the whole row on the first numeric value.
	var rows []evidenceRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	out := make([]agents.DuplicateEvidence, 0, len(rows))
	for _, r := range rows {
		out = append(out, agents.DuplicateEvidence{
			Field:  r.Field,
			Left:   r.Left.text(),
			Right:  r.Right.text(),
			Signal: r.Signal,
			Score:  r.Score,
		})
	}
	return out
}

// evidenceRow is one stored snapshot entry as it sits in the column.
type evidenceRow struct {
	Field  string        `json:"field"`
	Left   evidenceValue `json:"left_value"`
	Right  evidenceValue `json:"right_value"`
	Signal string        `json:"signal"`
	Score  float64       `json:"score"`
}

// evidenceValue is one compared value, which the snapshot writes as whatever
// the matching field held — a phone number is a string, a score is a number,
// and a one-sided signal writes JSON null for the side that has nothing.
//
// So it decodes ITSELF rather than being declared a string: a typed string
// member would fail the whole row on the first numeric value, and the row is
// the evidence a person reads before merging two records.
type evidenceValue struct{ s string }

func (v *evidenceValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v.s = s
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		v.s = strconv.FormatFloat(n, 'f', -1, 64)
		return nil
	}
	// Null, or a shape nothing writes. Empty, never the word "null" on
	// somebody's screen.
	v.s = ""
	return nil
}

func (v evidenceValue) text() string { return v.s }
