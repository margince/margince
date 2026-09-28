// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
)

const proposalRoute = "POST /v1/scheduling/proposals"

type replayRestore func(context.Context, string, string) (string, error)

func settleHTTPClaim(ctx context.Context, pool *pgxpool.Pool, hold claimHold, route string, rec *replayRecorder, wrote bool) error {
	body := rec.buf.String()
	if route == proposalRoute && rec.status >= 200 && rec.status < 300 {
		var response map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			return err
		}
		delete(response, "url")
		encoded, err := json.Marshal(response)
		if err != nil {
			return err
		}
		body = string(encoded)
	}
	return settleClaim(ctx, pool, hold, rec.status, body, rec.Header().Get("Content-Type"), 0, wrote)
}

func schedulingReplayRestore(store *activities.Store) replayRestore {
	return func(ctx context.Context, route, body string) (string, error) {
		if route != proposalRoute {
			return body, nil
		}
		id, err := recordIDAt(body, "id")
		if err != nil {
			return "", err
		}
		link, err := store.ProposalLink(ctx, id)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(link)
		return string(encoded), err
	}
}
