// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Recurring dates use projected occurrence dates so each year has a new key.
type dateFieldScanAdapter struct {
	svc *customfields.Service
}

var _ automation.DateFieldScan = dateFieldScanAdapter{}

func (a dateFieldScanAdapter) Candidates(ctx context.Context, object, column string, from, to time.Time, recurring bool, limit int) ([]automation.DateFieldAnchor, error) {
	candidates, err := a.svc.DateFieldCandidates(ctx, object, column, from, to, recurring, limit)
	if err != nil {
		// The scanner can skip a retired field without aborting other rules.
		if errors.Is(err, customfields.ErrUnknownDateColumn) {
			return nil, fmt.Errorf("%w: %w", automation.ErrDateFieldUnavailable, err)
		}
		return nil, err
	}
	out := make([]automation.DateFieldAnchor, len(candidates))
	for i, c := range candidates {
		out[i] = automation.DateFieldAnchor{
			Ref:    datasource.EntityRef{Type: datasource.EntityType(object), ID: c.EntityID},
			Anchor: c.OccurrenceDate,
		}
	}
	return out, nil
}

// Missing mailbox access must not be counted as customer silence.
func quietScanStore(db *database.DB) *activities.Store {
	return activities.NewStore(db).WithOwnerMailbox(activities.OwnerMailbox{
		CaughtUp:  capture.MailboxCaughtUpSQL,
		Providers: MailProviders(),
	})
}

// NewTimeScanner uses the worker's shared engine and candidate readers.
func NewTimeScanner(db *database.DB, log *slog.Logger) *automation.TimeScanner {
	return NewTimeScannerWithClock(db, time.Now, log)
}

// NewTimeScannerWithClock uses one clock for candidate windows and event anchors.
func NewTimeScannerWithClock(db *database.DB, now func() time.Time, log *slog.Logger) *automation.TimeScanner {
	engine := NewWorkflowEngine(db)
	pool := db.Pool()
	return automation.NewTimeScannerWithClock(engine,
		newQuietPopulationReader(db),
		dateFieldScanAdapter{svc: customfields.NewService(pool, nil)},
		now, log)
}
