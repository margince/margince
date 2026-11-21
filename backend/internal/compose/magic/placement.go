// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Records with no page of their own, placed on the record they belong to.
//
// An offer, a contract or a file has no page a reader can open, so a line about
// one names and links its deal, company, contact or project. The child's own
// read rule and the parent's row scope must both admit the row. A row that
// fails either is not served, and the reader is not told it exists.
//
// The line shows the sentence and the parent's name, never the child's images.
// A child type has no field allowlist and no field mask of its own, so its
// images could carry what the reader may not read.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// placement places one child type on one parent type.
type placement struct {
	// child is the audited entity type, also its table.
	child string
	// parent is one of the owner-scoped record types, also its table.
	parent string
	// on joins the child's alias `placed` to the parent's alias `e`. A
	// compile-time literal, never request text.
	on string
	// grant is the child's own object grant. Empty when the parent's grant is
	// the child's whole read rule.
	grant string
	// readable is the child's own row rule, when the parent's scope is not
	// all of it. Nil when it is.
	readable func(ctx context.Context, alias string, arg func(any) int) (string, error)
}

// The placed child types.
const (
	childOffer        = "offer"
	childContract     = "contract"
	childDealRoom     = "deal_room"
	childEvidence     = "deal_stage_evidence"
	childIntroduction = introductions.Table
	childAssignment   = "record_assignment"
	childAttachment   = "attachment"
)

// placements is every child type the done lane can place, in the order a row
// tries them. A contract with a deal is placed on the deal, one without on its
// company, as the contract's own visibility anchors it.
var placements = []placement{
	{child: childOffer, parent: typeDeal, grant: childOffer, on: "e.id = placed.deal_id"},
	{child: childContract, parent: typeDeal, grant: childContract, on: "e.id = placed.deal_id", readable: auth.ContractVisibleClause},
	{child: childContract, parent: typeCompany, grant: childContract, on: "e.id = placed.company_id AND placed.deal_id IS NULL", readable: auth.ContractVisibleClause},
	{child: childDealRoom, parent: typeDeal, grant: childDealRoom, on: "e.id = placed.deal_id"},
	{child: childEvidence, parent: typeDeal, on: "e.id = placed.deal_id"},
	{child: childIntroduction, parent: typeContact, grant: introductions.Object, on: "e.id = placed.contact_id", readable: introductions.ReadableSQL},
	{child: childAssignment, parent: typeDeal, on: "e.id = placed.deal_id"},
	{child: childAssignment, parent: typeCompany, on: "e.id = placed.company_id"},
	{child: childAssignment, parent: typeProject, on: "e.id = placed.project_id"},
	{child: childAttachment, parent: typeContact, on: "placed.entity_type = 'contact' AND e.id = placed.entity_id"},
	{child: childAttachment, parent: typeCompany, on: "placed.entity_type = 'company' AND e.id = placed.entity_id"},
	{child: childAttachment, parent: typeDeal, on: "placed.entity_type = 'deal' AND e.id = placed.entity_id"},
	{child: childAttachment, parent: typeLead, on: "placed.entity_type = 'lead' AND e.id = placed.entity_id"},
	{child: childAttachment, parent: typeProject, on: "placed.entity_type = 'project' AND e.id = placed.entity_id"},
}

// placedSentences name what a machine did to a child, by audit action. The
// line's subject is the parent's name, so each sentence ends where it goes.
var placedSentences = map[string]map[string]string{
	childOffer: {
		actionCreate: "magic.action.create_offer", actionUpdate: "magic.action.update_offer", actionArchive: "magic.action.archive_offer",
	},
	childContract: {
		actionCreate: "magic.action.create_contract", actionUpdate: "magic.action.update_contract", actionArchive: "magic.action.archive_contract",
	},
	childDealRoom: {
		actionCreate: "magic.action.create_deal_room", actionUpdate: "magic.action.update_deal_room", actionArchive: "magic.action.archive_deal_room",
	},
	childEvidence: {
		actionCreate: "magic.action.create_stage_evidence", actionUpdate: "magic.action.update_stage_evidence", actionArchive: "magic.action.archive_stage_evidence",
	},
	childIntroduction: {
		actionCreate: "magic.action.create_introduction", actionUpdate: "magic.action.update_introduction", actionArchive: "magic.action.archive_introduction",
	},
	childAssignment: {
		actionCreate: "magic.action.create_assignment", actionUpdate: "magic.action.update_assignment", actionArchive: "magic.action.archive_assignment",
	},
	childAttachment: {
		actionCreate: "magic.action.create_attachment", actionUpdate: "magic.action.update_attachment", actionArchive: "magic.action.archive_attachment",
	},
}

// armPlaced names the read placed children come from. One read serves every
// placement, so a cut marks all of their lines as a floor.
const armPlaced = "placed"

// describePlaced answers the sentence for a placed child's row. An update that
// moved only bookkeeping says nothing a reader can use, as on any record.
func describePlaced(e entry) (description, bool) {
	key, ok := placedSentences[e.EntityType][e.Action]
	if !ok {
		return description{}, false
	}
	if e.Action == actionUpdate && len(changedFields(e.EntityType, objectOf(e.Before), objectOf(e.After))) == 0 {
		return description{}, false
	}
	return description{summary: crmcontracts.MagicSentence{Key: key}}, true
}

// doneForPlacements reads the placed children's machine actions in ONE scan of
// the window. Each row joins the one placement its type names, and that join
// carries both gates. Up to fourteen reads of the window would be the
// alternative, each scanning the same rows.
func doneForPlacements(
	ctx context.Context, tx pgx.Tx, since time.Time, actions []string, width int,
) ([]entry, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sinceAt, actors, actionsAt := arg(since), arg(MachineActors()), arg(actions)
	branches, children, err := placementBranches(ctx, arg)
	if err != nil || len(branches) == 0 {
		return nil, err
	}
	unscrubbed := privacy.UnscrubbedImageSQL("a", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))
	where := fmt.Sprintf(
		`a.occurred_at >= $%d AND a.actor_type = ANY($%d) AND a.action = ANY($%d) AND a.entity_type = ANY($%d) AND %s AND %s`,
		sinceAt, actors, actionsAt, arg(children), unscrubbed, notRetention)
	where += completedCutoff(ctx, arg)
	// LIMIT 1 because a row whose child names two parents is still one line.
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, %s,
		       NULLIF(placed_on.label, ''), a.evidence, a.batch_id, NULL::text,
		       placed_on.type, placed_on.id
		  FROM audit_log a
		  CROSS JOIN LATERAL (%s LIMIT 1) placed_on
		 WHERE %s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, groupingImages, strings.Join(branches, " UNION ALL "), where, arg(width))
	return scanEntries(ctx, tx, query, args)
}

// placementBranches renders one lateral branch per placement the reader may
// read, and the child types they cover. A denied grant drops the branch and
// leaves the others alone, as a denied arm does in doneSince.
func placementBranches(ctx context.Context, arg func(any) int) ([]string, []string, error) {
	var branches, children []string
	seen := map[string]bool{}
	for _, p := range placements {
		branch, ok, err := p.branch(ctx, arg)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		branches = append(branches, branch)
		if !seen[p.child] {
			seen[p.child] = true
			children = append(children, p.child)
		}
	}
	return branches, children, nil
}

// branch renders the placement's select, or reports that the reader may not
// read it. Every predicate is the owner's own: the parent's scope and
// works-on clauses, and the child's grant and row rule.
func (p placement) branch(ctx context.Context, arg func(any) int) (string, bool, error) {
	for _, grant := range []string{p.parent, p.grant} {
		if grant == "" {
			continue
		}
		if err := auth.Require(ctx, grant, principal.ActionRead); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				return "", false, nil
			}
			return "", false, err
		}
	}
	predicates := []string{fmt.Sprintf("a.entity_type = '%s'", p.child), "placed.id = a.entity_id"}
	scope, err := auth.ScopeClauseFor(ctx, p.parent, "e", arg)
	if err != nil {
		return "", false, err
	}
	worksOn, err := auth.WorksOnClause(ctx, p.parent, "e", arg)
	if err != nil {
		return "", false, err
	}
	predicates = append(predicates, worksOn)
	if scope != "" {
		predicates = append(predicates, scope)
	}
	if p.readable != nil {
		own, err := p.readable(ctx, "placed", arg)
		if err != nil {
			return "", false, err
		}
		predicates = append(predicates, own)
	}
	return fmt.Sprintf(`SELECT '%s'::text AS type, e.id, (%s)::text AS label FROM %s placed JOIN %s e ON %s WHERE %s`,
		p.parent, recordLabels[p.parent], p.child, p.parent, p.on, strings.Join(predicates, " AND ")), true, nil
}

// placeableSQL is the predicate for an audit row `a` some placement can place,
// whoever reads. The unplaced count leaves these out, so placing a type takes
// it off System health's list.
func placeableSQL() string {
	matches := make([]string, 0, len(placements))
	for _, p := range placements {
		matches = append(matches, fmt.Sprintf(
			`(a.entity_type = '%s' AND EXISTS (SELECT 1 FROM %s placed JOIN %s e ON %s WHERE placed.id = a.entity_id))`,
			p.child, p.child, p.parent, p.on))
	}
	return "(" + strings.Join(matches, " OR ") + ")"
}
