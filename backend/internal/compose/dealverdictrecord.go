// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The verdict as the deal's receipt keeps it in verdict_actions. It names the
// owner it was made for, why no model was asked, what came of each source and
// of each action. A receipt written before this shape holds the bare list of
// actions, and both decode alike.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// Why a verdict asked no model.
const (
	verdictSkippedSwitchOff  = "switched_off"
	verdictSkippedNoEvidence = "no_evidence"
)

// The sources a verdict reads, in the order a reader sees them.
const (
	verdictSourceMail     = "mail"
	verdictSourceMeetings = "meetings"
	verdictSourceCalls    = "calls"
	verdictSourceNotes    = "notes"
	verdictSourceOffers   = "offers"
)

// What a verdict made of one source. Private is a source whose only items on
// the deal are ones not every member may read, so the verdict left them out.
// Unavailable is a mailbox that was not read on time.
const (
	sourceRead        = "read"
	sourcePrivate     = "private"
	sourceUnavailable = "unavailable"
	sourceNone        = "none"
)

// verdictSourceKinds maps each activity kind a verdict reads to its source.
var verdictSourceKinds = map[string]string{
	string(crmcontracts.ActivityKindEmail): verdictSourceMail, string(crmcontracts.ActivityKindMeeting): verdictSourceMeetings,
	string(crmcontracts.ActivityKindCall): verdictSourceCalls, string(crmcontracts.ActivityKindNote): verdictSourceNotes,
}

// verdictRecord is one verdict as stored.
type verdictRecord struct {
	// For is the owner the verdict was made for. Only they read why.
	For     string           `json:"for"`
	Skipped string           `json:"skipped,omitempty"`
	Sources []verdictSource  `json:"sources"`
	Actions []recordedAction `json:"actions"`
}

// verdictSource is one source and what the verdict made of it.
type verdictSource struct {
	Source string `json:"source"`
	State  string `json:"state"`
}

// decodeVerdictRecord reads either stored shape.
func decodeVerdictRecord(raw json.RawMessage) (verdictRecord, error) {
	var out verdictRecord
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return out, nil
	}
	if trimmed[0] == '[' {
		err := json.Unmarshal(trimmed, &out.Actions)
		return out, err
	}
	err := json.Unmarshal(trimmed, &out)
	return out, err
}

// verdictActionsSQL is the stored actions of either shape, as a JSON array.
func verdictActionsSQL(column string) string {
	return `(CASE WHEN jsonb_typeof(` + column + `) = 'array' THEN ` + column + ` ELSE ` + column + `->'actions' END)`
}

// mailReadFor answers whether the mailboxes that carried a deal's mail were
// read on time, as the close as lost asks it.
type mailReadFor func(owner *ids.UUID, mailboxes []string) bool

// readVerdictSources grades each source for one deal. Activity counts follow
// the verdict's own reading: engagement only, live, and split by whether every
// member may read them.
func readVerdictSources(
	ctx context.Context, tx pgx.Tx, c verdictCandidate, read dealVerdictRead, mailRead mailReadFor,
) ([]verdictSource, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.kind, count(*) FILTER (WHERE `+visibleToAllSQL("a")+`), count(*) FILTER (WHERE NOT `+visibleToAllSQL("a")+`)
		  FROM activity_link l JOIN activity a ON a.id = l.activity_id
		 WHERE l.deal_id = $1 AND a.archived_at IS NULL AND a.kind IN ('email', 'meeting', 'call', 'note')
		   AND `+relstrength.OriginIsEngagementSQL("a")+`
		 GROUP BY a.kind`, c.deal)
	if err != nil {
		return nil, fmt.Errorf("compose: counting what a verdict read: %w", err)
	}
	states := map[string]string{}
	for rows.Next() {
		var kind string
		var visible, private int
		if err := rows.Scan(&kind, &visible, &private); err != nil {
			rows.Close()
			return nil, err
		}
		states[verdictSourceKinds[kind]] = sourceState(visible, private)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var mailboxes []string
	if err := tx.QueryRow(ctx, `SELECT `+dealMailboxesSQL("d")+` FROM deal d WHERE d.id = $1`, c.deal).Scan(&mailboxes); err != nil {
		return nil, fmt.Errorf("compose: reading the mailboxes a verdict read: %w", err)
	}
	if owner := c.owner; !mailRead(&owner, mailboxes) {
		states[verdictSourceMail] = sourceUnavailable
	}
	states[verdictSourceOffers] = sourceState(len(read.in.Offers), 0)
	out := make([]verdictSource, 0, 5)
	for _, source := range []string{verdictSourceMail, verdictSourceMeetings, verdictSourceCalls, verdictSourceNotes, verdictSourceOffers} {
		state, ok := states[source]
		if !ok {
			state = sourceNone
		}
		out = append(out, verdictSource{Source: source, State: state})
	}
	return out, nil
}

func sourceState(visible, private int) string {
	switch {
	case visible > 0:
		return sourceRead
	case private > 0:
		return sourcePrivate
	default:
		return sourceNone
	}
}
