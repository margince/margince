// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// defaultReportWindowDays matches the launch gate's own window, so what a
// reader sees on the settings page is what the gate reads when it decides.
// Two different defaults would let a transition look ready on the screen and
// be refused by the policy, with nothing on either saying why.
const defaultReportWindowDays = 30

// GetStageAutomationReport answers what each transition in a pipeline has
// earned.
func (h Handlers) GetStageAutomationReport(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetStageAutomationReportParams,
) {
	days := defaultReportWindowDays
	if params.WindowDays != nil {
		days = *params.WindowDays
	}
	rates, err := h.store.ReadStageAutomationReport(r.Context(),
		ids.From[ids.PipelineKind](ids.UUID(params.PipelineId)),
		time.Duration(days)*24*time.Hour)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.StageAutomationReport{
		Data:       stageTransitionRecords(rates),
		WindowDays: days,
	})
}

// stageTransitionRecords renders the rates for the wire.
//
// The rates are computed from the counts server-side, by the same methods on
// TransitionRates that any other reader calls. A client dividing two of the
// counts itself would be writing a second definition of "clean acceptance" —
// and the definition that decides anything is the one on TransitionRates.
func stageTransitionRecords(rates []TransitionRates) []crmcontracts.StageTransitionRecord {
	out := make([]crmcontracts.StageTransitionRecord, 0, len(rates))
	for _, r := range rates {
		out = append(out, crmcontracts.StageTransitionRecord{
			PipelineId:          openapi_types.UUID(r.PipelineID.UUID),
			FromStageId:         openapi_types.UUID(r.FromStageID.UUID),
			ToStageId:           openapi_types.UUID(r.ToStageID.UUID),
			FromStageName:       r.FromName,
			ToStageName:         r.ToName,
			Reviewed:            r.Reviewed,
			Proposed:            r.Proposed,
			Expired:             r.Expired,
			Superseded:          r.Superseded,
			AcceptedClean:       r.AcceptedClean,
			AcceptedEdited:      r.AcceptedEdited,
			Rejected:            r.Rejected,
			AutoApplied:         r.AutoApplied,
			Unsafe:              r.Unsafe,
			ObservationDays:     r.ObservationDays,
			CleanAcceptanceRate: r.CleanAcceptanceRate(),
			EditRate:            r.EditRate(),
			RejectionRate:       r.RejectionRate(),
			UnsafeRate:          r.UnsafeRate(),
			EvidenceKinds:       evidenceKindRecords(r.EvidenceKinds),
		})
	}
	return out
}

func evidenceKindRecords(kinds []EvidenceKindRates) []crmcontracts.StageTransitionEvidenceRecord {
	out := make([]crmcontracts.StageTransitionEvidenceRecord, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, crmcontracts.StageTransitionEvidenceRecord{
			Kind:          k.Kind,
			Reviewed:      k.Reviewed,
			AcceptedClean: k.AcceptedClean,
			Unsafe:        k.Unsafe,
		})
	}
	return out
}
