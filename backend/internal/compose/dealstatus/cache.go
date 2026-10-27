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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Corrupt or obsolete payloads are cache misses; their row metadata stays separately readable.
type stored struct {
	ProjectionVersion string                      `json:"projection_version"`
	Fingerprint       string                      `json:"-"`
	GeneratedAt       time.Time                   `json:"-"`
	GeneratedBy       crmcontracts.WrittenBy      `json:"-"`
	Card              crmcontracts.DealStatusCard `json:"card"`
}

// The reader key prevents one seat from receiving another seat’s derived content.
func (s *Service) cached(ctx context.Context, userID ids.UserID, dealID ids.DealID) (stored, bool, error) {
	var out stored
	var payload []byte
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT fingerprint, generated_at, generated_by, payload FROM deal_status_card
			WHERE user_id = $1 AND deal_id = $2`,
			userID, dealID).Scan(&out.Fingerprint, &out.GeneratedAt, &out.GeneratedBy, &payload)
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
