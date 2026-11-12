// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// subjectRestrictions is the Router's restriction check, over the stops the
// consent module records, in the caller's workspace.
type subjectRestrictions struct{ pool *pgxpool.Pool }

func (s subjectRestrictions) AnyRestricted(ctx context.Context, subjects model.Subjects) (bool, error) {
	restricted := false
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		restricted, err = storekit.SubjectsRestricted(ctx, tx, subjects)
		return err
	})
	return restricted, err
}

// restrictionRecheck is how long a job whose subject is restricted waits
// before it asks again; the lift lets the next attempt through.
const restrictionRecheck = 6 * time.Hour

// snoozeWhileRestricted turns a restriction refusal into a snooze, so a job
// keeps its item and reads it once the restriction is lifted.
func snoozeWhileRestricted(err error) error {
	if errors.Is(err, ai.ErrSubjectRestricted) {
		return river.JobSnooze(restrictionRecheck)
	}
	return err
}

// documentSubjects names whose data a document carries: the record it is
// filed on, which stands for every contact on or linked to it.
func documentSubjects(parent ids.Ref) *model.Subjects {
	switch parent.Type {
	case ids.ContactID{}.EntityType():
		return model.AboutContacts(parent.ID)
	case ids.ActivityID{}.EntityType():
		return model.AboutActivities(parent.ID)
	case ids.CompanyID{}.EntityType():
		return &model.Subjects{Companies: []ids.UUID{parent.ID}}
	case ids.DealID{}.EntityType():
		return &model.Subjects{Deals: []ids.UUID{parent.ID}}
	case ids.ProjectID{}.EntityType():
		return &model.Subjects{Projects: []ids.UUID{parent.ID}}
	case ids.LeadID{}.EntityType():
		return model.AboutNoContact("a lead is not a contact, and a restriction names contacts only")
	}
	return &model.Subjects{}
}
