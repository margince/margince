// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HistoryImport identifies durable source attribution without coupling graph maintenance to a connector.
type HistoryImport struct {
	RunID         ids.UUID
	Active        bool
	AdditiveLinks bool
	SourceXID     string
	AddedContact  ids.UUID
}

// HistoryImportResolver locks the source run while its derived work is recorded.
type HistoryImportResolver func(context.Context, pgx.Tx, events.Envelope) (*HistoryImport, error)

// WithHistoryResolver coalesces historical arrivals; other changes retain the immediate exact fold.
func (g *GraphEdgeGen) WithHistoryResolver(resolve HistoryImportResolver) *GraphEdgeGen {
	g.historyResolver = resolve
	return g
}

const eventActivityCaptured = "activity.captured"

func graphArrivalCanWait(env events.Envelope) bool {
	if env.Type == eventActivityCaptured {
		return true
	}
	relinked, opened := graphArrivalUpdate(env)
	return relinked || opened
}

func graphArrivalUpdate(env events.Envelope) (bool, bool) {
	if env.Type != "activity.updated" {
		return false, false
	}
	var update struct {
		ChangedFields map[string]json.RawMessage `json:"changed_fields"`
	}
	if err := json.Unmarshal(env.Payload, &update); err != nil || len(update.ChangedFields) != 1 {
		return false, false
	}
	if raw, ok := update.ChangedFields["audience"]; ok {
		var audience string
		return false, json.Unmarshal(raw, &audience) == nil && audience == string(crmcontracts.Workspace)
	}
	raw, ok := update.ChangedFields["relinked"]
	if !ok {
		return false, false
	}
	var target *crmcontracts.PublicEventActivityRelinkedRef
	return json.Unmarshal(raw, &target) == nil && target != nil && ids.UUID(target.EntityId) != ids.Nil, false
}

type historyGraphCohort struct {
	run          ids.UUID
	includePrior bool
	addedContact ids.UUID
}

func (t *edgeTargets) addArrivals(ctx context.Context, tx pgx.Tx, resolve HistoryImportResolver, arrivals []events.Envelope) error {
	var captured, updated []ids.UUID
	groups := make(map[historyGraphCohort][]ids.UUID)
	sources := make(map[historyGraphCohort][]string)
	for _, env := range arrivals {
		var history *HistoryImport
		if resolve != nil {
			var err error
			history, err = resolve(ctx, tx, env)
			if err != nil {
				return err
			}
		}
		if history == nil {
			if env.Type == eventActivityCaptured {
				captured = append(captured, env.Entity.ID)
			} else {
				updated = append(updated, env.Entity.ID)
			}
			continue
		}
		relinked, _ := graphArrivalUpdate(env)
		cohort := historyGraphCohort{includePrior: relinked && !history.AdditiveLinks}
		if history.AdditiveLinks {
			cohort.addedContact = history.AddedContact
		}
		if history.Active {
			cohort.run = history.RunID
		}
		sources[cohort] = append(sources[cohort], history.SourceXID)
		groups[cohort] = append(groups[cohort], env.Entity.ID)
	}
	if err := t.addActivities(ctx, tx, captured, false); err != nil {
		return err
	}
	if err := t.addActivities(ctx, tx, updated, true); err != nil {
		return err
	}
	for cohort, activities := range groups {
		var targets edgeTargets
		if err := targets.addActivities(ctx, tx, activities, cohort.includePrior); err != nil {
			return err
		}
		var deferred *ids.UUID
		if cohort.run != ids.Nil {
			deferred = &cohort.run
		}
		if cohort.addedContact != ids.Nil {
			targets.keepContact(cohort.addedContact)
		}
		t.history.add(targets, deferred, sources[cohort])
	}
	return nil
}

func (t *edgeTargets) keepContact(contact ids.UUID) {
	peers := t.contactPairs[:0]
	for _, p := range t.contactPairs {
		if p.a == contact || p.b == contact {
			peers = append(peers, p)
		}
	}
	t.contactPairs = peers
	interactions := t.pairs[:0]
	for _, p := range t.pairs {
		if p.contact == contact {
			interactions = append(interactions, p)
		}
	}
	t.pairs = interactions
}
