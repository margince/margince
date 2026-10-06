// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The binding between automation rules and the lists they name. A rule reads
// a Live List's observed changes and adds to a Shortlist through the same
// collections store the list pages use, and a list's page reads which rules
// depend on it through the automation store; neither module imports the other.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ListRules is automation.Lists over the collections store.
type ListRules struct{ store *collections.Store }

var _ automation.Lists = ListRules{}

// NewListRules binds automation rules to the lists they name.
func NewListRules(pool *pgxpool.Pool) ListRules {
	return ListRules{store: NewCollectionsStore(pool)}
}

// Find reads a list the caller can find.
func (a ListRules) Find(ctx context.Context, id ids.UUID) (automation.ListRef, error) {
	l, err := a.store.ListForRule(ctx, ids.From[ids.ListKind](id))
	if err != nil {
		return automation.ListRef{}, err
	}
	return automation.ListRef{
		ID: l.ID.UUID, Name: l.Name, EntityType: l.EntityType, Live: l.Live,
		Archived: l.Archived, Version: l.Version, Invalid: l.Invalid,
	}, nil
}

// ObservedChanges reads what one check saw change, as the caller may see it.
func (a ListRules) ObservedChanges(ctx context.Context, id ids.UUID, version int64, checkedAt time.Time, actions []string, limit int) ([]automation.ListChange, int, error) {
	changes, total, err := a.store.ObservedChanges(ctx, ids.From[ids.ListKind](id), version, checkedAt, actions, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]automation.ListChange, 0, len(changes))
	for _, c := range changes {
		out = append(out, automation.ListChange{
			EventID: c.EventID, Action: c.Action,
			Record: datasource.EntityRef{Type: datasource.EntityType(c.EntityType), ID: c.EntityID},
		})
	}
	return out, total, nil
}

// CheckShortlist asks whether the caller may change the Shortlist.
func (a ListRules) CheckShortlist(ctx context.Context, id ids.UUID, entityType string) error {
	return a.store.CheckShortlistChange(ctx, ids.From[ids.ListKind](id), entityType)
}

// AddMember adds the record as an automation, admitted as the rule owner.
func (a ListRules) AddMember(ctx, admit context.Context, id ids.UUID, record datasource.EntityRef) (bool, error) {
	return a.store.AddMemberOnBehalf(ctx, admit, ids.From[ids.ListKind](id), collections.MemberChange{
		EntityType: string(record.Type), EntityID: record.ID, Reason: collections.ReasonAutomation,
	})
}

// ruleUsesOf reads, for a list's page, the active rules that depend on it.
func ruleUsesOf(pool *pgxpool.Pool) func(ctx context.Context, id ids.ListID) ([]collections.RuleUse, error) {
	rules := automation.NewAutomationStore(InstallationDB(pool))
	return func(ctx context.Context, id ids.ListID) ([]collections.RuleUse, error) {
		uses, err := rules.RulesOnList(ctx, id.UUID)
		if err != nil {
			return nil, err
		}
		out := make([]collections.RuleUse, 0, len(uses))
		for _, u := range uses {
			out = append(out, collections.RuleUse(u))
		}
		return out, nil
	}
}
