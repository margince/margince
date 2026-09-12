// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The send path's requirement edge, bound to the consent module.
//
// comms hands a message to a provider and must not know what a jurisdiction
// pack is; consent owns the packs and what the engine resolved each delivery to
// be. Neither imports the other, so the edge is injected here like every other
// cross-module edge.
//
// This is the caller gates/messagingruleapplied_test.go named as missing for
// SubjectPrefix: the label was declared, fixed by decree, and nothing between a
// composed subject and the provider ever looked at it.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
)

type requirementAdapter struct {
	store *consent.Store
}

// CheckMessage answers one finding per requirement the message does not meet.
//
// The message crosses the seam as data and the delivery id as a string, so the
// interface comms declares carries no type whose meaning consent owns.
func (a requirementAdapter) CheckMessage(
	ctx context.Context, m comms.FinalMessage,
) ([]comms.RequirementFinding, error) {
	owed, err := a.store.CheckMessage(ctx, m.DeliveryID, m.DecisionSetID, m.Subject)
	if err != nil {
		return nil, err
	}
	out := make([]comms.RequirementFinding, 0, len(owed))
	for _, f := range owed {
		out = append(out, comms.RequirementFinding{Requirement: f.Requirement, Detail: f.Detail})
	}
	return out, nil
}

// requirementCheckerFor builds the checker the send worker holds.
//
// WITH THE INSTALLATION COUNTRY, which is what selects the pack. A store
// without it resolves no jurisdiction and answers no findings, so the gate
// would pass every message and read exactly like a green one — the failure this
// whole check exists to end, reintroduced by a missing seam.
func requirementCheckerFor(pool *pgxpool.Pool) requirementAdapter {
	return requirementAdapter{store: consent.NewStore(InstallationDB(pool)).
		WithInstallationCountry(consent.InstallationCountryFunc(identity.CountryOf))}
}
