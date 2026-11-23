// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
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
	// FactsOnly marks a card a facts-only read wrote where a model was owed. A
	// facts-only read may serve it; an ordinary read treats it as a miss.
	FactsOnly bool `json:"facts_only,omitempty"`
}

// The reader key prevents one seat from receiving another seat’s derived content.
func (s *Service) cached(ctx context.Context, userID ids.UserID, dealID ids.DealID) (stored, bool, error) {
	var found map[ids.UUID]generatedsummary.Entry
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		found, err = readableCards(ctx, tx, userID, []ids.UUID{dealID.UUID})
		return err
	})
	row, cached := found[dealID.UUID]
	if err != nil || !cached {
		return stored{}, false, err
	}
	out := stored{
		Fingerprint: row.Fingerprint, GeneratedAt: row.GeneratedAt,
		GeneratedBy: crmcontracts.WrittenBy(row.GeneratedBy),
	}
	if !decodeStored(row.Payload, &out) {
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
	return generatedsummary.Store(ctx, s.pool, userID, generatedsummary.DealStatusCard(dealID),
		generatedsummary.Entry{
			Fingerprint: card.Fingerprint,
			GeneratedAt: card.GeneratedAt,
			GeneratedBy: string(card.GeneratedBy),
			Payload:     payload,
		})
}

func decodeStored(payload []byte, into *stored) bool {
	if err := json.Unmarshal(payload, into); err != nil {
		return false
	}
	return into.ProjectionVersion == ProjectionVersion
}

// readableCards answers the reader's stored cards for the deals that are live
// and inside the reader's row scope. A card outlives neither.
func readableCards(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, dealIDs []ids.UUID,
) (map[ids.UUID]generatedsummary.Entry, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := fmt.Sprintf("d.id = ANY($%d) AND d.archived_at IS NULL", arg(dealIDs))
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		where += " AND " + scope
	}
	rows, err := tx.Query(ctx, `SELECT d.id FROM deal d WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	readable, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(readable) == 0 {
		return nil, err
	}
	return generatedsummary.ReadEach(ctx, tx, userID, generatedsummary.KindDealStatusCard, readable)
}
