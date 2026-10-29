// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/profileverdicts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *Env) bindStores(db *database.DB) {
	e.Activities = activities.NewStore(db)
	// A no-op rename: the installation-name edge is compose's, which this package
	// cannot import, and compose's installationname suites prove it.
	// Merge readers use stored plan dates; no current-week calculation is needed.
	e.Contacts = contacts.NewStore(db).
		WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
		WithProviderRunReader(integrations.ReadRunForContactTx).
		WithMergeSignalReader(signals.PreviewSignalCarry(nil)).
		WithMergeCommitmentReader(weeklyplan.NewStore(db, nil, identityTeammates{svc: identity.NewServiceFor(db)}).PreviewCommitmentCarryTx).
		WithMergeNoticeReader(notices.PreviewNoticeCarryTx).
		WithMergeContentReader(e.Activities).
		WithProfileVerdictReader(profileverdicts.MergeReader(db)).
		WithFeedbackCarryReader(ai.NewFeedbackStore(db)).
		// As production wires it: a merge keeps the former owner's access.
		WithGrantCarrier(identity.NewServiceFor(db)).
		WithInstallationRenamer(func(context.Context, pgx.Tx, string) error { return nil })
	e.Deals = deals.NewStore(db, installseam.Deals())
	e.Projects = ProjectsStore(db)
	e.Contracts = ContractsStore(db, e.Deals)
}

type identityTeammates struct{ svc *identity.Service }

func (t identityTeammates) SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error) {
	return t.svc.SharesLiveTeamWithCaller(ctx, ids.From[ids.UserKind](other))
}
