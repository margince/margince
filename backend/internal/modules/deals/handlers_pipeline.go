// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListPipelines serves the pipeline catalog. The page is the whole catalog —
// a workspace holds a handful of pipelines — so the only dial is whether the
// archived ones come with it.
func (h Handlers) ListPipelines(w http.ResponseWriter, r *http.Request, params crmcontracts.ListPipelinesParams) {
	archived := storekit.LiveOnly
	if params.IncludeArchived != nil && *params.IncludeArchived {
		archived = storekit.IncludeArchived
	}
	pipelines, err := h.store.ListPipelines(r.Context(), archived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.PipelineListResponse{
		Data: pipelines,
		Page: crmcontracts.PageInfo{HasMore: false},
	})
}

func (h Handlers) CreatePipeline(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreatePipelineParams) {
	var req crmcontracts.CreatePipelineRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Name == "" {
		httperr.Write(w, r, httperr.Validation("name", "required", "name is required"))
		return
	}

	in := CreatePipelineInput{
		Name:      req.Name,
		IsDefault: req.IsDefault != nil && *req.IsDefault,
	}
	if req.Position != nil {
		in.Position = *req.Position
	}
	if req.Stages != nil {
		for i, st := range *req.Stages {
			stage := StageInput{Name: st.Name, Position: i + 1, Semantic: string(SemanticOpen)}
			if st.Position != nil && *st.Position != 0 {
				stage.Position = *st.Position
			}
			if st.Semantic != nil {
				stage.Semantic = string(*st.Semantic)
			}
			stage.WinProbability = stageProbability(stage.Semantic, st.WinProbability)
			in.Stages = append(in.Stages, stage)
		}
	}

	pipeline, err := h.store.CreatePipeline(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/pipelines/"+pipeline.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, pipeline)
}

// ReorderPipelines answers the live catalog in its new order, the same page
// listPipelines serves, so the caller holds what every picker now shows.
func (h Handlers) ReorderPipelines(w http.ResponseWriter, r *http.Request, _ crmcontracts.ReorderPipelinesParams) {
	var req crmcontracts.PipelineOrderRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.PipelineIds == nil {
		httperr.Write(w, r, httperr.Validation("pipeline_ids", "required", "pipeline_ids is required"))
		return
	}
	pipelines, err := h.store.ReorderPipelines(r.Context(), uuidArgs(&req.PipelineIds))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.PipelineListResponse{
		Data: pipelines,
		Page: crmcontracts.PageInfo{HasMore: false},
	})
}

// ReorderStages answers the pipeline with its stages in their new order, pinned
// by If-Match to the ladder the caller drew the order from.
func (h Handlers) ReorderStages(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ReorderStagesParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.StageOrderRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.StageIds == nil {
		httperr.Write(w, r, httperr.Validation("stage_ids", "required", "stage_ids is required"))
		return
	}
	pipeline, err := h.store.ReorderStages(r.Context(), pathID[ids.PipelineKind](id), uuidArgs(&req.StageIds), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, pipeline)
}

func (h Handlers) GetPipeline(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	pipeline, err := h.store.GetPipeline(r.Context(), pathID[ids.PipelineKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, pipeline)
}
