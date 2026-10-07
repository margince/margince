// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The per-task request settings surface: read, judge and save the thinking
// level and timeouts an admin sets per task. Thin transport; the ai store owns
// the RBAC gate, the bounds and the audit-only write.

import (
	"encoding/json"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func (h aiRoutingHandlers) GetAiTaskOverrides(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiTaskOverrides")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	stored, err := h.store.GetTaskOverrides(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeTaskOverrides(w, stored)
}

func (h aiRoutingHandlers) ReplaceAiTaskOverrides(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ReplaceAiTaskOverrides")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.AiTaskOverrides
	if !httperr.Decode(w, r, &req) {
		return
	}
	next, err := ai.TaskOverridesFromWire(req, sentTask(r))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	expected, err := routingPrecondition(r.Header)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	stored, err := h.store.ReplaceTaskOverrides(r.Context(), next, expected)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeTaskOverrides(w, stored)
}

func (h aiRoutingHandlers) PreviewAiTaskOverrides(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "PreviewAiTaskOverrides")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.AiTaskOverrides
	if !httperr.Decode(w, r, &req) {
		return
	}
	// A field the transport refuses is one more problem in the preview's list.
	next, refused := ai.TaskOverridesFromWire(req, sentTask(r))
	preview, err := h.store.PreviewTaskOverrides(r.Context(), next, refused)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, taskOverridesPreviewToWire(preview))
}

// sentTask is each task's value as the client wrote it, from the body Decode kept.
func sentTask(r *http.Request) func(string) (json.RawMessage, bool) {
	return func(task string) (json.RawMessage, bool) { return httperr.PresentField(r, task) }
}

func writeTaskOverrides(w http.ResponseWriter, stored ai.TaskOverrides) {
	w.Header().Set("ETag", `"`+stored.Revision()+`"`)
	httperr.WriteJSON(w, http.StatusOK, taskOverridesToWire(stored))
}

func taskOverridesToWire(v ai.TaskOverrides) crmcontracts.AiTaskOverrides {
	out := crmcontracts.AiTaskOverrides{}
	for task, o := range v {
		out[string(task)] = o.Wire()
	}
	return out
}

func taskOverridesPreviewToWire(p ai.TaskOverridesPreview) crmcontracts.AiTaskOverridesPreview {
	out := crmcontracts.AiTaskOverridesPreview{Effective: map[string]crmcontracts.AiTaskSettings{}, Stale: []string{}}
	for task, settings := range p.Effective {
		out.Effective[string(task)] = taskSettingsToWire(settings)
	}
	for _, task := range p.Stale {
		out.Stale = append(out.Stale, string(task))
	}
	out.Errors = p.Errors
	return out
}

func taskSettingsToWire(s ai.EffectiveTask) crmcontracts.AiTaskSettings {
	return crmcontracts.AiTaskSettings{
		Thinking:          optionalString(s.Thinking),
		DecisionTimeoutMs: int(s.DecisionTimeout.Milliseconds()),
		AttemptTimeoutMs:  int(s.AttemptTimeout.Milliseconds()),
	}
}
