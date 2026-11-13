// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func (s *TimeScanner) scanDateInstance(ctx context.Context, handler workflow.Handler, instance automationInstance, workspace ids.UUID, now time.Time, paramsFor dateFieldScanExtractor) error {
	var params struct {
		Recipient string `json:"recipient"`
	}
	if err := json.Unmarshal(instance.params, &params); err != nil {
		return err
	}
	reader := ctx
	if params.Recipient != "" {
		var err error
		reader, err = ownerContext(ctx, s.engine.resolver, workspace, instance.owner)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return scanDateFieldInstanceCandidates(reader, s.dateScan, handler, instance, workspace, now,
		func(_ context.Context, h workflow.Handler, event workflow.Event) error {
			return s.engine.runOne(ctx, h, event)
		}, paramsFor)
}
