// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordsources holds the data tasks that move records whose source
// predates the fixed list of channels onto a channel.
package recordsources

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// correct moves one page of a table's rows, and count says how many are left.
type (
	correct func(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error)
	count   func(ctx context.Context, q datatask.Querier, maximum int64) (int64, error)
)

// DataTask corrects one table per step. The audit row of each corrected record
// keeps the source it held, so the system and id an old value packed stay readable.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795363335_record_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795363335_record_source_channels"),
		Requires: []string{"core/1795363335"},
		Steps: []datatask.Step{
			step("deal_sources", deals.CorrectDealSourcesTx, deals.CountUnlistedDealSources),
			step("offer_sources", deals.CorrectOfferSourcesTx, deals.CountUnlistedOfferSources),
			step("project_sources", projects.CorrectProjectSourcesTx, projects.CountUnlistedProjectSources),
		},
	}
}

// SignalDataTask corrects signals, whose older source named the producer. The
// producer moves to source_system, where the scan's readers select on it.
func SignalDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795589886_signal_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795589886_signal_source_channels"),
		Requires: []string{"core/1795589886"},
		Steps: []datatask.Step{
			step("signal_sources", signals.CorrectSignalSourcesTx, signals.CountUnlistedSignalSources),
		},
	}
}

// PartyDataTask corrects contacts and companies. A connector or writer an older
// source named moves to source_system, so a captured row keeps its mailbox.
func PartyDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795624398_party_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795624398_party_source_channels"),
		Requires: []string{"core/1795624398"},
		Steps: []datatask.Step{
			step("contact_sources", contacts.CorrectContactSourcesTx, contacts.CountUnlistedContactSources),
			step("company_sources", contacts.CorrectCompanySourcesTx, contacts.CountUnlistedCompanySources),
		},
	}
}

// LeadDataTask moves every lead whose source is off the fixed channel list onto
// its channel, through the mapping contacts use.
func LeadDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795710028_lead_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795710028_lead_source_channels"),
		Requires: []string{"core/1795710028"},
		Steps: []datatask.Step{
			step("lead_sources", contacts.CorrectLeadSourcesTx, contacts.CountUnlistedLeadSources),
		},
	}
}

// ActivityDataTask moves every activity whose source is off the fixed channel
// list onto its channel. The writer an older word named moves to source_system.
func ActivityDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795771059_activity_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795771059_activity_source_channels"),
		Requires: []string{"core/1795771059"},
		Steps: []datatask.Step{
			step("activity_sources", activities.CorrectActivitySourcesTx, activities.CountUnlistedActivitySources),
		},
	}
}

func step(name string, move correct, left count) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 500, MinLimit: 1,
		Audit:         datatask.AuditPolicy{PerRecord: true},
		EmptyWhenDone: true,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			unlisted, err := left(ctx, q, maximum)
			return min(unlisted, maximum), unlisted > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			moved, err := move(ctx, tx, after, in.Limit, in.RunID)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, moved, len(moved))
		},
	}
}
