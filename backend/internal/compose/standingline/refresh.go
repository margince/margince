// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ActivityReader is the activities store's gated reads, run as the system.
type ActivityReader interface {
	GetActivity(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error)
	ListActivities(ctx context.Context, in activities.ListActivitiesInput) ([]crmcontracts.Activity, storekit.Page, error)
}

// Refresher checks each record an activity landed on.
type Refresher struct {
	pool           *pgxpool.Pool
	activities     ActivityReader
	lane           Completer
	routingVersion func() string
	now            func() time.Time
}

// NewRefresher binds the reads and the summarize lane a line is written with.
func NewRefresher(
	pool *pgxpool.Pool, reader ActivityReader, lane Completer, routingVersion func() string, now func() time.Time,
) *Refresher {
	return &Refresher{pool: pool, activities: reader, lane: lane, routingVersion: routingVersion, now: now}
}

// listedActivities is read a little past RecentActivities, so a withheld row
// does not leave the line short.
const listedActivities = RecentActivities + 3

// Refresh checks the line of every contact, company and project the activity
// is filed on.
func (r *Refresher) Refresh(ctx context.Context, activityID ids.ActivityID) error {
	activity, err := r.activities.GetActivity(ctx, activityID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	if activity.Links == nil {
		return nil
	}
	lang := identity.BaseLanguageForPrompt(ctx, r.pool)
	for _, link := range *activity.Links {
		kind := string(link.EntityType)
		if kind != KindContact && kind != KindCompany && kind != KindProject {
			continue
		}
		record := generatedsummary.StandingRecord{EntityType: kind, EntityID: ids.UUID(link.EntityId)}
		if err := r.check(ctx, record, lang); err != nil {
			return err
		}
	}
	return nil
}

// check rewrites one record's line when its newest activities moved and the
// model says the shown line no longer holds.
func (r *Refresher) check(ctx context.Context, record generatedsummary.StandingRecord, lang string) error {
	limit := listedActivities
	rows, _, err := r.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &record.EntityType, EntityID: &record.EntityID, Limit: &limit,
	})
	if err != nil {
		return err
	}
	var shown generatedsummary.StandingLine
	var found bool
	if err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		shown, found, err = generatedsummary.ReadStandingLineTx(ctx, tx, record)
		return err
	}); err != nil {
		return err
	}
	in := FromActivities(record.EntityType, shown.Text, rows)
	if len(in.Recent) == 0 {
		return nil
	}
	fingerprint, err := Fingerprint(in, r.routingVersion(), lang)
	if err != nil || (found && shown.Fingerprint == fingerprint) {
		return err
	}
	answer, err := Write(ai.WithSubject(ctx, ids.Ref{Type: record.EntityType, ID: record.EntityID}, ""), r.lane, in, lang)
	if err != nil {
		return fmt.Errorf("check the status line of a %s: %w", record.EntityType, err)
	}
	return database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		if answer.Text == "" {
			// Kept, or too little to say: the line stands and this input is read.
			return generatedsummary.TouchStandingLineTx(ctx, tx, record, fingerprint)
		}
		evidence := make([]ids.UUID, len(answer.Cited))
		for i, cited := range answer.Cited {
			parsed, err := ids.Parse(cited)
			if err != nil {
				return fmt.Errorf("cite an activity in a status line: %w", err)
			}
			evidence[i] = parsed
		}
		return generatedsummary.SaveStandingLineTx(ctx, tx, record, generatedsummary.StandingLine{
			Text: answer.Text, Evidence: evidence, Language: lang,
			Fingerprint: fingerprint, WrittenAt: r.now().UTC(),
		})
	})
}
