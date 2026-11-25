// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// retireUnlisted hands a complete, settled listing's event ids to the sink,
// which retires the stored meetings between from and until that it left out.
// One event whose id will not read makes the listing incomplete. Then nothing
// is retired, and settled is false so the re-list stays due.
func retireUnlisted(ctx context.Context, sink connector.Sink, events [][]byte, from, until time.Time) (settled bool, err error) {
	retirer, ok := sink.(connector.UnlistedMeetingRetirer)
	if !ok {
		return true, nil
	}
	listed := make([]string, 0, len(events))
	for _, raw := range events {
		var ev struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &ev); err != nil || ev.ID == "" {
			return false, nil //nolint:nilerr // an unreadable id leaves the listing's word incomplete, which retires nothing
		}
		listed = append(listed, ev.ID)
	}
	return true, retirer.RetireUnlistedMeetings(ctx, connectorName, listed, from, until)
}
