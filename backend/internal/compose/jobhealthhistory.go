// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type jobHistoryCursor struct {
	Workspace, Reader, Kind, State string
	Days                           int
	Cutoff                         time.Time
	Before                         int64
}

func (h jobHealthHandlers) GetJobHealthHistory(w http.ResponseWriter, r *http.Request, p crmcontracts.GetJobHealthHistoryParams) {
	if !admitHealthReader(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), jobHealthReadTimeout)
	defer cancel()
	ws, ok := principal.WorkspaceID(ctx)
	actor, bound := principal.Actor(ctx)
	if !ok || !bound {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	now := utcNow()
	cursor := jobHistoryCursor{Workspace: ws.String(), Reader: actor.ID, Days: 7, Cutoff: now}
	if p.Kind != nil {
		cursor.Kind = *p.Kind
	}
	if p.State != nil {
		cursor.State = string(*p.State)
	}
	if p.Days != nil {
		cursor.Days = *p.Days
	}
	if err := jobHistoryFilterError(cursor); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if p.Cursor != nil {
		previous, err := decodeJobHistoryCursor(*p.Cursor, cursor, now)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		cursor = previous
	}
	from := cursor.Cutoff.AddDate(0, 0, -cursor.Days)
	result, err := jobs.WorkspaceHistory(ctx, h.pool, jobs.HistoryFilter{Workspace: cursor.Workspace, Dispatchers: dispatcherKinds(), Kind: cursor.Kind, State: cursor.State, From: from, Cutoff: cursor.Cutoff, BeforeID: cursor.Before})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.JobHealthHistory{ObservedAt: now, Cutoff: cursor.Cutoff, WindowStart: from, Coverage: "retained_only", HasMore: result.More, PopulationMayHaveChanged: p.Cursor != nil, Groups: []crmcontracts.JobHealthGroup{}, Runs: jobFailuresResponse(result.Runs)}
	for _, g := range result.Groups {
		out.Groups = append(out.Groups, crmcontracts.JobHealthGroup{Kind: g.Kind, State: crmcontracts.JobHealthGroupState(g.State), FleetWide: g.FleetWide, Count: g.Count, FirstAt: g.FirstAt, LatestAt: g.LatestAt})
	}
	if result.More {
		cursor.Before = result.Runs[len(result.Runs)-1].ID
		next, err := storekit.EncodeOpaque(cursor)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		out.NextCursor = &next
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func jobHistoryFilterError(cursor jobHistoryCursor) error {
	switch {
	case cursor.Days < 1 || cursor.Days > 7:
		return httperr.Validation("days", "out_of_range", "choose a window from one to seven days")
	case len(cursor.Kind) > 200:
		return httperr.Validation("kind", "too_long", "use a job kind of at most 200 characters")
	case cursor.State != "" && cursor.State != string(crmcontracts.GetJobHealthHistoryParamsStateRetryable) && cursor.State != string(crmcontracts.GetJobHealthHistoryParamsStateDiscarded) && cursor.State != string(crmcontracts.GetJobHealthHistoryParamsStateCancelled):
		return httperr.Validation("state", "invalid_state", "choose retryable, discarded or cancelled")
	default:
		return nil
	}
}

func decodeJobHistoryCursor(encoded string, current jobHistoryCursor, now time.Time) (jobHistoryCursor, error) {
	invalid := httperr.Validation("cursor", "invalid_cursor", "refresh the history to start a new traversal")
	if len(encoded) > 2048 {
		return jobHistoryCursor{}, invalid
	}
	previous, err := storekit.DecodeOpaque[jobHistoryCursor](encoded)
	if err != nil {
		return jobHistoryCursor{}, invalid
	}
	if !previous.sameFilters(current) || previous.Cutoff.After(now) || previous.Cutoff.Before(now.AddDate(0, 0, -7)) || previous.Before <= 0 {
		return jobHistoryCursor{}, invalid
	}
	return previous, nil
}

func (cursor jobHistoryCursor) sameFilters(other jobHistoryCursor) bool {
	return cursor.Workspace == other.Workspace && cursor.Reader == other.Reader &&
		cursor.Kind == other.Kind && cursor.State == other.State && cursor.Days == other.Days
}
