// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// One mailbox's history baseline: a local date before which the mail it
// brought in suggests no automatic commitments. The mail stays captured and
// searchable; only the automatic suggestion is withheld.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// HistoryBaseline is the date a mailbox's automatic suggestions start from.
type HistoryBaseline struct {
	// Date is the chosen calendar day at UTC midnight, as a date column reads back.
	Date     time.Time
	Timezone string
	SetBy    string
	SetAt    time.Time
}

// Excludes reports whether an instant falls on a local day strictly before the
// baseline date. A message sent on the date itself is not excluded.
func (b HistoryBaseline) Excludes(at time.Time) (bool, error) {
	zone, err := storekit.LoadZone(b.Timezone)
	if err != nil {
		return false, err
	}
	return storekit.WorkspaceDay(at, zone).Before(b.Date), nil
}

// SetHistoryBaseline sets or, with a nil date, clears the calling human's own
// baseline for one mailbox. It completes, deletes and rewrites nothing else.
func (r *Registry) SetHistoryBaseline(ctx context.Context, name string, date *time.Time, timezone string) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ConnectionView{}, fmt.Errorf("capture: only a human sets a mailbox's history baseline: %w", apperrors.ErrPermissionDenied)
	}
	if _, ok := r.connectors[name]; !ok {
		return ConnectionView{}, ErrNoConnection
	}
	var after *HistoryBaseline
	if date != nil {
		if err := values.ZoneName(timezone); err != nil {
			return ConnectionView{}, err
		}
		after = &HistoryBaseline{Date: storekit.AsDate(*date), Timezone: timezone, SetBy: actor.ID, SetAt: time.Now().UTC()}
	}
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var before historyBaselineColumns
		// The before-image comes out of the statement that replaces it, as the
		// sibling mailbox settings do.
		err := tx.QueryRow(ctx, `
			UPDATE capture_connection
			   SET history_baseline_date = $3, history_baseline_tz = $4,
			       history_baseline_set_by = $5, history_baseline_set_at = $6
			 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL
			RETURNING (SELECT c.history_baseline_date FROM capture_connection c WHERE c.id = capture_connection.id),
			          (SELECT c.history_baseline_tz FROM capture_connection c WHERE c.id = capture_connection.id)`,
			append([]any{actor.UserID, name}, historyBaselineArgs(after)...)...).Scan(&before.date, &before.timezone)
		if err != nil {
			return err
		}
		// Audit-only, like the other per-mailbox settings: the public event
		// catalog carries no type for a mailbox's own setting.
		_, err = storekit.Audit(ctx, tx, "update", captureSettingsObject, storekit.MustWorkspace(ctx),
			historyBaselineImage(name, before.baseline()), historyBaselineImage(name, after))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, ErrNoConnection
	}
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: setting the mailbox's history baseline: %w", err)
	}
	return r.connectionFor(ctx, name)
}

// historyBaselineColumns scans the four nullable columns one baseline is stored in.
type historyBaselineColumns struct {
	date     *time.Time
	timezone *string
	setBy    *string
	setAt    *time.Time
}

func (c historyBaselineColumns) baseline() *HistoryBaseline {
	if c.date == nil || c.timezone == nil {
		return nil
	}
	b := &HistoryBaseline{Date: *c.date, Timezone: *c.timezone}
	if c.setBy != nil {
		b.SetBy = *c.setBy
	}
	if c.setAt != nil {
		b.SetAt = *c.setAt
	}
	return b
}

func historyBaselineArgs(b *HistoryBaseline) []any {
	if b == nil {
		return []any{nil, nil, nil, nil}
	}
	return []any{b.Date, b.Timezone, b.SetBy, b.SetAt}
}

// historyBaselineImage names the mailbox, because a seat may hold several and
// the date alone would not say which one moved.
func historyBaselineImage(provider string, b *HistoryBaseline) map[string]any {
	image := map[string]any{auditKeyProvider: provider, "history_baseline_date": nil, "history_baseline_tz": nil}
	if b != nil {
		image["history_baseline_date"] = b.Date.Format(time.DateOnly)
		image["history_baseline_tz"] = b.Timezone
	}
	return image
}
