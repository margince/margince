// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Corrupt or obsolete payloads are cache misses; their row metadata stays separately readable.
type stored struct {
	ProjectionVersion string                      `json:"projection_version"`
	Access            *cardAccess                 `json:"access,omitempty"`
	Fingerprint       string                      `json:"-"`
	GeneratedAt       time.Time                   `json:"-"`
	GeneratedBy       crmcontracts.WrittenBy      `json:"-"`
	Card              crmcontracts.DealStatusCard `json:"card"`
}

// The reader key prevents one seat from receiving another seat’s derived content.
func (s *Service) cached(ctx context.Context, userID ids.UserID, dealID ids.DealID) (stored, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return stored{}, false, err
	}
	var out stored
	var payload []byte
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		where, args, err := cachedDealScope(ctx, userID, []ids.UUID{dealID.UUID})
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT c.fingerprint, c.generated_at, c.generated_by, c.payload
			FROM deal_status_card c JOIN deal d ON d.id = c.deal_id WHERE `+where, args...).
			Scan(&out.Fingerprint, &out.GeneratedAt, &out.GeneratedBy, &payload)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, false, nil
	}
	if err != nil {
		return stored{}, false, err
	}
	if !decodeStored(payload, &out) {
		return stored{}, false, nil
	}
	return out, true, nil
}

func (s *Service) save(ctx context.Context, userID ids.UserID, dealID ids.DealID, card stored) error {
	// Message summaries carry reader access and must never enter the shared payload.
	card.ProjectionVersion = ProjectionVersion
	briefevidence.Strip(cardEvidence(&card.Card))
	payload, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("encode the deal status payload: %w", err)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO deal_status_card (user_id, deal_id, fingerprint,
			                              generated_at, generated_by, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, deal_id) DO UPDATE
			SET fingerprint = EXCLUDED.fingerprint,
			    generated_at = EXCLUDED.generated_at,
			    generated_by = EXCLUDED.generated_by,
			    payload = EXCLUDED.payload`,
			userID, dealID, card.Fingerprint, card.GeneratedAt, card.GeneratedBy, payload)
		return err
	})
}

func decodeStored(payload []byte, into *stored) bool {
	if err := json.Unmarshal(payload, into); err != nil {
		return false
	}
	return into.ProjectionVersion == ProjectionVersion
}

func cachedDealScope(ctx context.Context, userID ids.UserID, dealIDs []ids.UUID) (string, []any, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := fmt.Sprintf("c.user_id = $%d AND d.id = ANY($%d) AND d.archived_at IS NULL", arg(userID), arg(dealIDs))
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if scope != "" {
		where += " AND " + scope
	}
	return where, args, err
}
