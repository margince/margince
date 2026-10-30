// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assuranceCallExceptions reads the open findings about standing calls. A
// call finding reaches only a seat that may see the call itself, which is a
// seat that may assert about the call's population (forecasting.MayAssertAbout).
// requested narrows to calls about that population alone. A read for named
// owners is a read of their deals' findings, which a call is not.
func assuranceCallExceptions(
	ctx context.Context, tx pgx.Tx, owners []ids.UUID, requested *RequestedScope,
) ([]assurance.Exception, error) {
	if owners != nil || !auth.ReadGranted(ctx, objectForecast) {
		return nil, nil
	}
	args := []any{assurance.SubjectKindForecastCall}
	arg := func(v any) int { args = append(args, v); return len(args) }
	narrow := ""
	if requested != nil {
		narrow += fmt.Sprintf(" AND c.scope_kind = $%d AND c.scope_id IS NOT DISTINCT FROM $%d",
			arg(requested.Kind), arg(requested.ID))
	}
	rows, err := tx.Query(ctx, `
		SELECT `+exceptionSelect(rawFindingMoney)+`, c.scope_kind, c.scope_id
		  FROM assurance_exception e
		  JOIN forecast_call c ON c.id = e.subject_id
		 WHERE e.status = 'open' AND e.subject_kind = $1`+narrow+`
		 ORDER BY e.last_seen_at DESC, e.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the call findings: %w", err)
	}
	type callFinding struct {
		exception assurance.Exception
		scope     forecasting.Scope
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (callFinding, error) {
		var out callFinding
		e := &out.exception
		err := row.Scan(&e.ID, &e.Type, &e.SubjectKind, &e.SubjectID, &e.Severity,
			&e.AffectedMinor, new(*string), &e.OwnerID, &e.Status,
			&e.Claim, &e.Observed, &e.FirstSeenAt, &e.LastSeenAt, &out.scope.Kind, &out.scope.ID)
		return out, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the call findings: %w", err)
	}
	var out []assurance.Exception
	for _, f := range found {
		visible, err := forecasting.MayAssertAbout(ctx, tx, ForecastWritableScope, f.scope)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		e, err := narrowCallFinding(ctx, tx, f.exception, f.scope)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// slotWithheld marks a call finding that left out deals its reader cannot read.
const slotWithheld = "withheld"

// callFindingDealLimit is how many moved deals a call finding names.
const callFindingDealLimit = 20

// narrowCallFinding holds a call finding to what this reader may see. The
// pass stored it as the system, over every deal of the scope. The deals go
// through forecastSnapshotLens, the rule a frozen snapshot is read by. When
// one is not readable, only the comparison and the readable deals are kept:
// every count and amount covers the withheld ones too. A reader with any mask
// on a deal's amount gets no money, whatever the deals hold today.
func narrowCallFinding(
	ctx context.Context, tx pgx.Tx, e assurance.Exception, scope forecasting.Scope,
) (assurance.Exception, error) {
	var observed map[string]any
	if err := json.Unmarshal(e.Observed, &observed); err != nil {
		return e, fmt.Errorf("compose: reading a call finding: %w", err)
	}
	raw, named := observed["deal_ids"].([]any)
	if !named {
		return e, nil
	}
	rows := make([]forecasting.Contribution, 0, len(raw))
	for _, id := range raw {
		if deal, ok := id.(string); ok {
			rows = append(rows, forecasting.Contribution{DealID: deal})
		}
	}
	readable, err := forecastSnapshotLens(ctx, tx, scope, nil, rows)
	if err != nil {
		return e, err
	}
	shown := make([]string, 0, len(readable))
	for _, row := range readable {
		shown = append(shown, row.DealID)
	}
	shown = shown[:min(len(shown), callFindingDealLimit)]
	if len(readable) < len(raw) {
		observed = map[string]any{"comparison": observed["comparison"], "deal_ids": shown, slotWithheld: true}
	}
	observed["deal_ids"] = shown
	masked, err := auth.MasksAnyRowOf(ctx, tableDeal, "amount_minor")
	if err != nil {
		return e, err
	}
	if masked {
		for key := range observed {
			if strings.HasSuffix(key, "_change_minor") {
				delete(observed, key)
			}
		}
	}
	e.Observed, err = json.Marshal(observed)
	return e, err
}
