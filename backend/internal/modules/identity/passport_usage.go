// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PassportUsageReader reads the running window of each subject
// (agentvolume.SubjectOf) in one round trip. compose wires the meter.
type PassportUsageReader interface {
	Usage(ctx context.Context, ws ids.UUID, subjects []string) (map[string]agentvolume.Usage, error)
	ResetsAt(ctx context.Context) time.Time
}

// WithPassportUsage wires the meter the passport list reports usage from.
// Without it the list carries no usage.
func (h Handlers) WithPassportUsage(r PassportUsageReader) Handlers {
	h.passportUsage = r
	return h
}

// liveUsage answers the usage of every live row, keyed by subject. A failure
// is logged and answers nothing: the list is still worth showing without it.
func (h Handlers) liveUsage(ctx context.Context, rows []PassportRow, now time.Time) map[string]agentvolume.Usage {
	ws, bound := principal.WorkspaceID(ctx)
	if h.passportUsage == nil || !bound {
		return nil
	}
	subjects := make([]string, 0, len(rows))
	for _, p := range rows {
		if p.live(now) {
			subjects = append(subjects, p.UsageSubject())
		}
	}
	usage, err := h.passportUsage.Usage(ctx, ws, subjects)
	if err != nil {
		slog.WarnContext(ctx, "the passport list is answered without usage; the agent counters could not be read", "err", err)
		return nil
	}
	return usage
}

func toAgentUsage(u agentvolume.Usage) crmcontracts.AgentUsage {
	counter := func(c agentvolume.CounterUsage) crmcontracts.AgentCounterUsage {
		return crmcontracts.AgentCounterUsage{Used: c.Used, Limit: c.Limit, Released: c.Released}
	}
	return crmcontracts.AgentUsage{
		Writes: counter(u.Writes), Egress: counter(u.Egress), Calls: counter(u.Calls),
		ResetsAt: u.ResetsAt, State: crmcontracts.AgentUsageState(u.State()),
	}
}
