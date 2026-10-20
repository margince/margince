// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type dataTaskHandlers struct {
	classifier *OwedClassifier
	pool       *pgxpool.Pool
	clock      func() time.Time
}

func (h dataTaskHandlers) now() time.Time {
	if h.clock != nil {
		return h.clock()
	}
	return time.Now()
}

func (h dataTaskHandlers) ListDataTasks(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionRead) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	engine, err := newDataTaskEngine(h.pool, buildinfo.ReleaseVersion, h.now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	progress, err := engine.List(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.DataTaskProgress `json:"data"`
	}{Data: DataTaskProgressReport(progress)})
}

func (h dataTaskHandlers) ControlDataTask(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionUpdate) {
		return
	}
	var in crmcontracts.DataTaskControl
	if !httperr.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	engine, err := newDataTaskEngine(h.pool, buildinfo.ReleaseVersion, h.now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := engine.Control(ctx, in.TaskId, in.Step, string(in.Action), in.Version); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if in.Action == "resume" || in.Action == "retry" {
		if err := enqueueDataTaskReconcile(ctx, h.pool); err != nil {
			httperr.Write(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// DataTaskProgressReport keeps the CLI and HTTP report on the same safe fields.
func DataTaskProgressReport(progress []datatask.Progress) []crmcontracts.DataTaskProgress {
	out := make([]crmcontracts.DataTaskProgress, 0, len(progress))
	for _, p := range progress {
		out = append(out, dataTaskProgressWire(p))
	}
	return out
}

func dataTaskProgressWire(p datatask.Progress) crmcontracts.DataTaskProgress {
	out := crmcontracts.DataTaskProgress{
		TaskId: p.TaskID, Title: p.Title, Step: p.Step, TaskVersion: p.TaskVersion,
		Status: crmcontracts.DataTaskProgressStatus(p.Status), Version: p.Version, BatchLimit: p.BatchLimit,
		Scanned: p.Scanned, Changed: p.Changed, Batches: p.Batches, Estimate: p.Estimate, EstimateCapped: p.EstimateCapped,
		EstimatedAt: p.EstimatedAt, ConsecutiveFailures: p.ConsecutiveFailures, LastErrorClass: p.LastErrorClass,
		NextAttemptAt: p.NextAttemptAt, LastProgressAt: p.LastProgressAt, MissingPrerequisites: append([]string{}, p.MissingPrerequisites...),
	}
	if p.TaskID == homeRepairTaskID {
		kind := crmcontracts.DataTaskProgressReviewKind("home_attention")
		out.ReviewKind = &kind
	}
	if !p.RunID.IsZero() {
		run := openapi_types.UUID(p.RunID)
		out.RunId = &run
	}
	if p.FirstRelease != "" {
		out.FirstRelease = &p.FirstRelease
	}
	if p.BlockedReason != "" {
		reason := crmcontracts.DataTaskProgressBlockedReason(p.BlockedReason)
		out.BlockedReason = &reason
	}
	if p.BlockedBy != "" {
		out.BlockedBy = &p.BlockedBy
	}
	if p.LastErrorClass != nil {
		if detail, ok := jobs.VettedFailure("data_task_run", jobs.SentenceForClass(*p.LastErrorClass)); ok {
			out.ErrorSentence, out.ErrorRemedy = &detail.Sentence, &detail.Remedy
		}
	}
	return out
}
