// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

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
	var subjects []agentvolume.Holder
	for _, holder := range holders {
		for _, subject := range holder.Subjects {
			subjects = append(subjects, agentvolume.Holder{Subject: subject, Roles: holder.Roles})
		}
	}
	usage, err := h.passportUsage.Usage(r.Context(), ws, subjects)
	if err != nil {
		slog.WarnContext(r.Context(), "agent usage is not answered; the agent counters could not be read", "err", err)
		if errors.Is(err, agentvolume.ErrUnanswerable) {
			httperr.ServiceUnavailable(w, r, "This server counts no agent usage, so there is none to show.")
			return
		}
		httperr.ServiceUnavailable(w, r, "The agent counters cannot be read right now. Try again shortly.")
		return
	}
	data := make([]crmcontracts.AgentUserUsage, 0, len(holders))
	for _, holder := range holders {
		data = append(data, userUsage(holder, usage, h.passportUsage.AllowanceFor(r.Context(), holder.Roles)))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AgentUsageByUser{
		Data: data, ResetsAt: h.resetsAt(r.Context(), usage),
	})
}

// userUsage sums a user's subjects into one row. Its state is the worst any
// one subject is in, since one stopped agent is stopped whatever the sum.
func userUsage(holder AgentUsageHolder, usage map[string]agentvolume.Usage, allowance agentvolume.Allowance) crmcontracts.AgentUserUsage {
	var total agentvolume.Usage
	state := agentvolume.UsageOK
	for _, subject := range holder.Subjects {
		total = total.Plus(usage[subject])
		state = state.Worst(usage[subject].State())
	}
	wire := toAgentUsage(total)
	roles := holder.Roles
	if roles == nil {
		roles = []string{}
	}
	return crmcontracts.AgentUserUsage{
		UserId: openapi_types.UUID(holder.UserID), DisplayName: holder.DisplayName,
		PassportCount: len(holder.Subjects),
		Writes:        wire.Writes, Egress: wire.Egress, Calls: wire.Calls,
		State:    crmcontracts.AgentUsageState(state),
		RoleKeys: roles, Allowance: toResolvedAllowance(allowance),
	}
}

// toResolvedAllowance reports one passport's allowance and the role each count
// comes from; an empty role is the installation value and answers as null.
func toResolvedAllowance(a agentvolume.Allowance) crmcontracts.AgentRoleResolvedAllowance {
	from := func(c agentvolume.Counter) crmcontracts.AgentAllowanceFrom {
		out := crmcontracts.AgentAllowanceFrom{Limit: a.Limits.Of(c)}
		if role := a.From[c]; role != "" {
			out.RoleKey = &role
		}
		return out
	}
	return crmcontracts.AgentRoleResolvedAllowance{
		Writes: from(agentvolume.Writes), Egress: from(agentvolume.Egress), Calls: from(agentvolume.Calls),
	}
}

// resetsAt takes the reset from the window the counts were read in. Only a read
// with no subjects asks the meter again, since it counted nothing.
func (h Handlers) resetsAt(ctx context.Context, usage map[string]agentvolume.Usage) time.Time {
	for _, u := range usage {
		return u.ResetsAt
	}
	return h.passportUsage.ResetsAt(ctx)
}
