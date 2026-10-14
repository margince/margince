// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListAgentUsageByUser implements (GET /ai/agent-usage): how much of their
// allowances each user's agents have used, and nothing about which agents.
func (h Handlers) ListAgentUsageByUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "agent usage is read by a signed-in human")
		return
	}
	holders, err := h.svc.AgentUsageHolders(r.Context(), identity)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	ws, bound := principal.WorkspaceID(r.Context())
	if h.passportUsage == nil || !bound {
		httperr.ServiceUnavailable(w, r, "Agent usage cannot be read on this server. Try again shortly.")
		return
	}
	var subjects []string
	for _, holder := range holders {
		subjects = append(subjects, holder.Subjects...)
	}
	usage, err := h.passportUsage.Usage(r.Context(), ws, subjects)
	if err != nil {
		httperr.ServiceUnavailable(w, r, "The agent counters cannot be read right now. Try again shortly.")
		return
	}
	data := make([]crmcontracts.AgentUserUsage, 0, len(holders))
	for _, holder := range holders {
		data = append(data, userUsage(holder, usage))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AgentUsageByUser{
		Data: data, ResetsAt: h.passportUsage.ResetsAt(r.Context()),
	})
}

// userUsage sums a user's subjects into one row. Its state is the worst any
// one subject is in, since one stopped agent is stopped whatever the sum.
func userUsage(holder AgentUsageHolder, usage map[string]agentvolume.Usage) crmcontracts.AgentUserUsage {
	var total agentvolume.Usage
	state := agentvolume.UsageOK
	for _, subject := range holder.Subjects {
		total = total.Plus(usage[subject])
		state = state.Worst(usage[subject].State())
	}
	wire := toAgentUsage(total)
	return crmcontracts.AgentUserUsage{
		UserId: openapi_types.UUID(holder.UserID), DisplayName: holder.DisplayName,
		PassportCount: len(holder.Subjects),
		Reads:         wire.Reads, Writes: wire.Writes, Egress: wire.Egress, Calls: wire.Calls,
		State: crmcontracts.AgentUsageState(state),
	}
}
