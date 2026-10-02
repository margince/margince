// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The per-task request settings surface: read, judge and save the thinking
// level and timeouts an admin sets per task. Thin transport; the ai store owns
// the RBAC gate, the bounds and the audit-only write.

import (
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
	expected, err := routingPrecondition(r.Header)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	stored, err := h.store.ReplaceTaskOverrides(r.Context(), taskOverridesFromWire(req), expected)
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
	preview, err := h.store.PreviewTaskOverrides(r.Context(), taskOverridesFromWire(req))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, taskOverridesPreviewToWire(preview))
}

func writeTaskOverrides(w http.ResponseWriter, stored ai.TaskOverrides) {
	w.Header().Set("ETag", `"`+stored.Revision()+`"`)
	httperr.WriteJSON(w, http.StatusOK, taskOverridesToWire(stored))
}

func taskOverridesToWire(v ai.TaskOverrides) crmcontracts.AiTaskOverrides {
	out := crmcontracts.AiTaskOverrides{}
	for task, o := range v {
		out[string(task)] = taskOverrideToWire(o)
	}
	return out
}

// taskOverrideToWire leaves a zero field absent: zero is "keep the product's
// value", which the contract spells as a missing key.
func taskOverrideToWire(o ai.TaskOverride) crmcontracts.AiTaskOverride {
	return crmcontracts.AiTaskOverride{
		Thinking:          optionalEnum[crmcontracts.AiTaskOverrideThinking](o.Thinking),
		DecisionTimeoutMs: optionalInt(o.DecisionTimeoutMs),
		AttemptTimeoutMs:  optionalInt(o.AttemptTimeoutMs),
	}
}

func taskOverridesFromWire(v crmcontracts.AiTaskOverrides) ai.TaskOverrides {
	out := ai.TaskOverrides{}
	for task, o := range v {
		var next ai.TaskOverride
		if o.Thinking != nil {
			next.Thinking = string(*o.Thinking)
		}
		if o.DecisionTimeoutMs != nil {
			next.DecisionTimeoutMs = *o.DecisionTimeoutMs
		}
		if o.AttemptTimeoutMs != nil {
			next.AttemptTimeoutMs = *o.AttemptTimeoutMs
		}
		out[ai.Task(task)] = next
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
