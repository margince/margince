// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func taskRecommendationReader(pool *pgxpool.Pool) activities.TaskRecommendation {
	return taskRecommendationFrom(newCurrentAdviceService(pool, time.Now))
}

func taskRecommendationFrom(service *dealstatus.Service) activities.TaskRecommendation {
	return func(ctx context.Context, tx pgx.Tx, receipt crmcontracts.RecommendationReceipt) (activities.LogActivityInput, error) {
		move, err := service.ValidateRecommendationTx(ctx, tx, receipt)
		if err != nil {
			return activities.LogActivityInput{}, err
		}
		if move.Action != dealstatus.ActionCreateTask || move.Arguments == nil {
			return activities.LogActivityInput{}, fmt.Errorf("refresh the changed suggested step: %w", apperrors.ErrVersionSkew)
		}
		encoded, err := json.Marshal(*move.Arguments)
		if err != nil {
			return activities.LogActivityInput{}, fmt.Errorf("encode the current suggested task: %w", err)
		}
		var request crmcontracts.CreateTaskRequest
		if err := json.Unmarshal(encoded, &request); err != nil {
			return activities.LogActivityInput{}, fmt.Errorf("decode the current suggested task: %w", err)
		}
		return activities.TaskInputFrom(request)
	}
}
