// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwnerTargets reads each owner's own target for metric over window, in the
// one statement an evaluation reads its targets with and under the same reach.
// readable is false when the reader may not read targets at all, so a caller
// can tell a withheld target from one nobody set. The reader must measure
// scope and every owner, as an owner attainment chart must.
func (s *Service) OwnerTargets(
	ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, metric crmcontracts.ReportingMetricID,
	window crmcontracts.ReportingWindow, owners []ids.UUID,
) (targets map[ids.UUID]int64, readable bool, err error) {
	if !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return nil, false, nil
	}
	targets = map[ids.UUID]int64{}
	if len(owners) == 0 {
		return targets, true, nil
	}
	reach, err := s.targetReach(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	frame := crmcontracts.ReportingContext{Scope: scope, TargetInterval: &window}
	keys := make([]targetKey, 0, len(owners))
	subjects := make([]openapi_types.UUID, 0, len(owners))
	for _, owner := range owners {
		subject := openapi_types.UUID(owner)
		keys, subjects = append(keys, ownerKey(subject, metric)), append(subjects, subject)
	}
	held, err := s.matchingTargets(ctx, tx, reach, frame, keys, subjects)
	if err != nil {
		return nil, false, err
	}
	for index, owner := range owners {
		if target, ok := held[keys[index]]; ok {
			targets[owner] = target.Definition.Value
		}
	}
	return targets, true, nil
}
