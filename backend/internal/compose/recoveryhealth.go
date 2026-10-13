// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// recoveryHealthHandlers serve the restore-drill evidence to Settings → System
// health. The ledger is written only by margince-migrate's drill verbs, so this
// surface reads and never writes.
type recoveryHealthHandlers struct {
	ledger *continuity.Store
	now    func() time.Time
}

// GetRecoveryHealth reports the most recent drill against the published targets.
func (h recoveryHealthHandlers) GetRecoveryHealth(w http.ResponseWriter, r *http.Request) {
	if !admitHealthReader(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	drill, ever, err := h.ledger.Latest(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "recovery health read failed", "err", err)
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.RecoveryHealth{
		GeneratedAt:           h.now(),
		RecoveryTargetSeconds: int(continuity.RecoveryTarget.Seconds()),
		DataLossTargetSeconds: int(continuity.DataLossTarget.Seconds()),
	}
	if ever {
		out.LastDrill = restoreDrillReport(drill)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// restoreDrillReport takes both windows from the drill's own methods, which
// derive them from the row's timestamps.
func restoreDrillReport(d continuity.Drill) *crmcontracts.RestoreDrill {
	report := &crmcontracts.RestoreDrill{
		StartedAt:       d.StartedAt,
		FinishedAt:      d.FinishedAt,
		RestoredTo:      d.RestoredTo,
		Outcome:         crmcontracts.RestoreDrillOutcome(d.Outcome),
		Operator:        d.Operator,
		DataLossSeconds: int(d.DataLossWindow().Seconds()),
	}
	if d.Notes != "" {
		notes := d.Notes
		report.Notes = &notes
	}
	if window, known := d.RecoveryWindow(); known {
		seconds := int(window.Seconds())
		report.RecoverySeconds = &seconds
	}
	return report
}
