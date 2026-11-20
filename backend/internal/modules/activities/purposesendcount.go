// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// defaultSendCountDays is the window the consent purposes card shows.
const defaultSendCountDays = 30

// PurposeSendCounts counts the outbound messages that named each consent
// purpose in the last `days` days. It reads every send in the installation, so
// it asks for the compliance read grant rather than any record's row scope.
func (s *Store) PurposeSendCounts(ctx context.Context, days int) ([]crmcontracts.ConsentPurposeSendCount, error) {
	if err := auth.Require(ctx, "consent_config", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.ConsentPurposeSendCount{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT consent_purpose, consent_class, count(*)
			FROM activity
			WHERE consent_purpose IS NOT NULL
			  AND direction = 'outbound'
			  AND occurred_at >= now() - make_interval(days => $1)
			GROUP BY consent_purpose, consent_class
			ORDER BY count(*) DESC, consent_purpose, consent_class`, days)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var count crmcontracts.ConsentPurposeSendCount
			if err := rows.Scan(&count.PurposeKey, &count.Class, &count.Sends); err != nil {
				return err
			}
			out = append(out, count)
		}
		return rows.Err()
	})
	return out, err
}

// ListConsentPurposeSendCounts serves GET /consent-purposes/send-counts.
func (h Handlers) ListConsentPurposeSendCounts(w http.ResponseWriter, r *http.Request, params crmcontracts.ListConsentPurposeSendCountsParams) {
	days := defaultSendCountDays
	if params.Days != nil {
		days = *params.Days
	}
	counts, err := h.store.PurposeSendCounts(r.Context(), days)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.ConsentPurposeSendCount `json:"data"`
		Days int                                    `json:"days"`
	}{counts, days})
}
