// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Hop 2 of the context walk: the records that share an activity with the
// anchor, rendered one related_* section per record type.

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// relatedSectionOrder is EVERY arm of activity_link, in the order the walk
// emits its related_* sections. It decides the order and nothing else: which
// arms are walked is activityLinkArms, and the two hold the same set.
//
// It used to be a shorter list, and the shortening was not a decision anybody
// made about leads — it was a list that predated the arm. The cost landed one
// hop away: activityLinkArms' own doc records that the dereference borrowed
// this list and so dropped the lead arm, which is the discovery call a prep is
// most often for. A second list of what is walkable is a list somebody has to
// remember, and this module has already paid for forgetting it once.
//
// A LEAD IS REPORTED, and the argument that placed it last as a SUBJECT is the
// argument for naming it here. Subject precedence puts a lead below the contact
// it may become, because an event naming both has a promoted record to prepare
// against. At hop 2 nothing is being chosen: the section is the room. So the
// lead is precisely the record the reader was NOT prepared against, and leaving
// it out means an unqualified lead sat on the discovery call and the catch-up
// said the room held four names. Bounded like every other leg — it appears
// only where it shares an activity with the anchor — and last, because a
// promoted record is the more useful one to read first.
//
// Held by: TestEveryLinkArmHasARelatedSection
// (backend/internal/modules/search/graphactivity_test.go), which reads the
// DDL's own enum rather than the sibling list in Go.
var relatedSectionOrder = []string{
	string(datasource.EntityContact),
	string(datasource.EntityCompany),
	string(datasource.EntityDeal),
	string(datasource.EntityProject),
	string(datasource.EntityLead),
}

func (s *Store) relatedViaLinks(ctx context.Context, tx pgx.Tx, anchorType string, anchorID ids.UUID, activityIDs []ids.ActivityID, maxItems int) ([]graphSection, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	sectionsByType := map[string][]graphItem{}
	for _, hop := range activityLinkArms {
		if hop.entity == anchorType {
			continue // the anchor is not its own neighbor
		}
		// Object RBAC hides a denied type SILENTLY here, unlike at the anchor:
		// a neighbor is context the caller did not ask for by name, so a type
		// they hold no grant on is absent rather than a 403 on a read they did
		// not make. Search's branch admission takes the same posture.
		if auth.Require(ctx, hop.entity, principal.ActionRead) != nil {
			continue
		}
		items, err := hopNeighbors(ctx, tx, hop, anchorID, activityIDs)
		if err != nil {
			return nil, err
		}
		sectionsByType[hop.entity] = items
	}
	var out []graphSection
	for _, entity := range relatedSectionOrder {
		items := sectionsByType[entity]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].id.String() < items[j].id.String() })
		if len(items) > maxItems {
			items = items[:maxItems]
		}
		out = append(out, graphSection{name: "related_" + pluralRelationName(entity), items: items})
	}
	return out, nil
}

// hopNeighbors reads one hop's bounded, deterministic candidate window and
// returns the visible ones as graph items. Each candidate is
// visibility-probed individually: the walk widens context, never authority.
func hopNeighbors(ctx context.Context, tx pgx.Tx, hop activityLinkArm, anchorID ids.UUID, activityIDs []ids.ActivityID) ([]graphItem, error) {
	branch, ok := hop.branch()
	if !ok {
		// An arm whose record type no branch declares has no table to read and
		// no way to render itself. The arm gate refuses that before it ships,
		// so reaching it here means the two have come apart at runtime — which
		// is a fault to report, never a neighbour type to drop silently. No
		// sentinel: nothing the caller did produced it, so it is a 500 and the
		// message stays in the log rather than on the wire.
		return nil, fmt.Errorf("search: no search branch declares %q, so its hop-2 read cannot be composed", hop.entity)
	}
	// Bounded like the activity leg: the id order makes the window
	// deterministic before the per-row visibility probe thins it.
	//
	// The title is composed as a select EXPRESSION, unqualified: a branch may
	// render its record from several columns (a lead does), and prefixing an
	// alias onto that reads as a schema name.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT t.id, %s
		FROM activity_link l JOIN %s t ON t.id = l.%s
		WHERE l.activity_id = ANY($1) AND t.archived_at IS NULL AND l.%s IS NOT NULL AND t.id <> $2
		ORDER BY t.id LIMIT %d`,
		branch.title, branch.table, hop.column, hop.column, graphExpansionLimit), activityIDs, anchorID)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id    ids.UUID
		title string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.title); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var items []graphItem
	for _, c := range candidates {
		visible, err := auth.VisibleTo(ctx, tx, hop.entity, c.id)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		items = append(items, graphItem{entityType: hop.entity, id: c.id, summary: c.title})
	}
	return items, nil
}
