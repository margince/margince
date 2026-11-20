// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// UnknownPurposeKey groups every send whose key names no live purpose. A
	// key the caller typed is never echoed back.
	UnknownPurposeKey = "unknown"
	// sendCountKeyLimit bounds the distinct stored keys one read groups.
	sendCountKeyLimit     = 200
	defaultSendCountDays  = 30
	sendCountDayDuration  = 24 * time.Hour
	sendCountDisputedSQL  = `requested_category IS NOT NULL AND requested_category <> resolved_category`
	sendCountDecisionsSQL = `
		SELECT coalesce(purpose_key, ''), count(DISTINCT delivery_id),
		       count(DISTINCT delivery_id) FILTER (WHERE ` + sendCountDisputedSQL + `)
		  FROM communication_decision
		 WHERE phase = 'staging' AND decided_at >= $1
		 GROUP BY 1
		 ORDER BY 2 DESC, 1
		 LIMIT $2`
)

// sendCount is one stored purpose key's staged messages in the window.
type sendCount struct {
	key             string
	sends, disputed int
}

// PurposeSendCounts counts the messages the engine staged under each purpose in
// the last `days` days. It also counts those whose claimed kind the record did
// not bear out. It reads the engine's own decision rows, so a send counts under
// what the engine judged, whatever door it came through.
func (s *Store) PurposeSendCounts(ctx context.Context, days int) ([]crmcontracts.ConsentPurposeSendCount, error) {
	if err := auth.Require(ctx, "consent_config", principal.ActionRead); err != nil {
		return nil, err
	}
	since := s.now().Add(-time.Duration(days) * sendCountDayDuration)
	var out []crmcontracts.ConsentPurposeSendCount
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		stored, err := stagedSendCounts(ctx, tx, since)
		if err != nil {
			return err
		}
		out, err = countsByLivePurpose(ctx, tx, stored)
		return err
	})
	return out, err
}

func stagedSendCounts(ctx context.Context, tx pgx.Tx, since time.Time) ([]sendCount, error) {
	rows, err := tx.Query(ctx, sendCountDecisionsSQL, since, sendCountKeyLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sendCount
	for rows.Next() {
		var c sendCount
		if err := rows.Scan(&c.key, &c.sends, &c.disputed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// countsByLivePurpose resolves each stored key the way the engine resolves a
// send's key, so an archived purpose and a typed word both read as unknown.
func countsByLivePurpose(ctx context.Context, tx pgx.Tx, stored []sendCount) ([]crmcontracts.ConsentPurposeSendCount, error) {
	at := map[string]int{}
	out := []crmcontracts.ConsentPurposeSendCount{}
	for _, c := range stored {
		key, class := UnknownPurposeKey, (*crmcontracts.ConsentPurposeClass)(nil)
		if c.key != "" {
			purpose, defined, err := purposeRowFor(ctx, tx, c.key)
			if err != nil {
				return nil, err
			}
			if defined {
				live := crmcontracts.ConsentPurposeClass(purpose.Class)
				key, class = purpose.Key, &live
			}
		}
		i, seen := at[key]
		if !seen {
			i = len(out)
			at[key] = i
			out = append(out, crmcontracts.ConsentPurposeSendCount{PurposeKey: key, Class: class})
		}
		out[i].Sends += c.sends
		out[i].Disputed += c.disputed
	}
	slices.SortStableFunc(out, func(a, b crmcontracts.ConsentPurposeSendCount) int {
		return cmp.Or(cmp.Compare(b.Sends, a.Sends), cmp.Compare(a.PurposeKey, b.PurposeKey))
	})
	return out, nil
}

// ListConsentPurposeSendCounts serves GET /consent-purposes/send-counts.
func (h Handlers) ListConsentPurposeSendCounts(w http.ResponseWriter, r *http.Request, params crmcontracts.ListConsentPurposeSendCountsParams) {
	days := defaultSendCountDays
	if params.Days != nil {
		days = *params.Days
	}
	counts, err := h.store.PurposeSendCounts(r.Context(), days)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.ConsentPurposeSendCount `json:"data"`
		Days int                                    `json:"days"`
	}{counts, days})
}
