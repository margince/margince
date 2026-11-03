// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StageChance is one open stage's typed chance, its recorded evidence and the
// chance the forecast uses for it.
type StageChance struct {
	StageID     ids.UUID
	Typed       int
	Evidence    StageEvidence
	ChanceInUse int
}

// StageChances holds the open stages' chances, as of one evidence cutoff.
type StageChances struct {
	SelfCorrecting bool
	EvidenceCutoff time.Time
	Stages         []StageChance
}

// StageChancesFunc reads the stage chances as of an instant. Injected, because
// the stages and their history belong to other modules.
type StageChancesFunc func(ctx context.Context, tx pgx.Tx, at time.Time) (StageChances, error)

// WithStageChances serves GET /forecast/stage-chances through read.
func (h Handlers) WithStageChances(read StageChancesFunc) Handlers {
	h.stageChances = read
	return h
}

// GetForecastStageChances answers the chance in use for every open stage.
func (h Handlers) GetForecastStageChances(w http.ResponseWriter, r *http.Request) {
	var out crmcontracts.ForecastStageChances
	err := h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
			return err
		}
		chances, err := h.stageChances(ctx, tx, h.now())
		if err != nil {
			return err
		}
		out = stageChancesToWire(chances)
		return nil
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func stageChancesToWire(in StageChances) crmcontracts.ForecastStageChances {
	out := crmcontracts.ForecastStageChances{
		SelfCorrecting: in.SelfCorrecting, EvidenceCutoff: in.EvidenceCutoff,
		PriorStrength: PriorStrength, Stages: make([]crmcontracts.ForecastStageChance, 0, len(in.Stages)),
	}
	for _, stage := range in.Stages {
		row := crmcontracts.ForecastStageChance{
			StageId: openapi_types.UUID(stage.StageID), TypedProbability: stage.Typed,
			EvidenceN: stage.Evidence.Resolved, ChanceInUse: stage.ChanceInUse,
		}
		if stage.Evidence.Resolved > 0 {
			row.WonPercent = new(int(math.Round(stage.Evidence.WonShare * 100)))
		}
		out.Stages = append(out.Stages, row)
	}
	return out
}
