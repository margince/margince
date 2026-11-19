// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity a member files on a deal is filed on the deal's project too. The
// project's timeline then holds it, and the account's other projects leave it
// out. The rule, for every door that files on a deal:
//
//   - Only a member's own write inherits. An agent, a connector, the system and
//     an import file no project this way.
//   - A project the caller named wins, and deals of two projects add none.
//   - The inherited link is marked (provenance.InheritedProjectRowSQL). The
//     mark keeps it from stamping retention, from widening who may change the
//     activity, and from counting as a human filing when the deal is declined.
//   - A deal that later changes project moves none of its earlier activities.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// inheritedProjectEvidence is the mark on the audit row of an inherited link.
func inheritedProjectEvidence() map[string]any {
	return map[string]any{provenance.ProjectRungKey: provenance.ProjectRungDeal}
}

// inheritedRelink is true for the relink that writes an inherited link.
func inheritedRelink(in RelinkActivityInput) bool {
	return in.EntityType == linkEntityProject && in.evidence[provenance.ProjectRungKey] == provenance.ProjectRungDeal
}

// inheritedProjectOf resolves, without a lock, the project the deals among
// links carry. It is zero when links name a project, or when the caller is not
// a member holding activity.update.
func inheritedProjectOf(ctx context.Context, tx pgx.Tx, links []ActivityLinkInput) (ids.UUID, error) {
	var deals []ids.UUID
	for _, link := range links {
		switch link.EntityType {
		case linkEntityProject:
			return ids.Nil, nil
		case linkEntityDeal:
			deals = append(deals, link.EntityID)
		}
	}
	if actor, ok := principal.Actor(ctx); len(deals) == 0 || !ok || actor.Type != principal.PrincipalHuman {
		return ids.Nil, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return ids.Nil, skipRefusal(err)
	}
	return storekit.DealsProject(ctx, tx, deals)
}

// linkLockTargets is what a write of links and their inherited project locks,
// in one sorted call, before any attach probe takes a share lock.
func linkLockTargets(links []ActivityLinkInput, project ids.UUID) storekit.LastActivityTargets {
	targets := lastActivityTargets(links)
	if !project.IsZero() {
		targets.Projects = append(targets.Projects, project)
	}
	return targets
}

// skipRefusal turns a refusal into "no inherited link": the caller asked to
// file on the deal, and that filing stands without the project.
func skipRefusal(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}

// mayAttachInherited asks of the inherited link what any link is asked. A
// refusal answers false.
func mayAttachInherited(ctx context.Context, tx pgx.Tx, project ids.UUID) (bool, error) {
	err := mayAttach(ctx, tx, ActivityLinkInput{EntityType: linkEntityProject, EntityID: project})
	return err == nil, skipRefusal(err)
}

// inheritDealProject resolves the project an activity about to be logged
// inherits. An automatic link never spends the caller's link allowance.
func inheritDealProject(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if len(in.Links) >= maxActivityLinks || importedLog(ctx, *in) {
		return nil
	}
	project, err := inheritedProjectOf(ctx, tx, in.Links)
	in.inheritedProject = project
	return err
}

// insertInheritedProject writes the inherited link of a new activity and its
// marked audit row. It stamps no retention: the deal's own qualification does.
func insertInheritedProject(ctx context.Context, tx pgx.Tx, id ids.ActivityID, project ids.UUID) error {
	if project.IsZero() {
		return nil
	}
	if ok, err := mayAttachInherited(ctx, tx, project); err != nil || !ok {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, project_id) VALUES ($1, 'project', $2)`,
		id, project); err != nil {
		return err
	}
	_, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", id.UUID, nil,
		map[string]any{fieldEntityType: linkEntityProject, fieldEntityID: project}, inheritedProjectEvidence())
	return err
}

// lockDealRelink resolves the project a relink onto a deal inherits, and
// locks the deal and that project in one call before any attach probe runs.
func lockDealRelink(ctx context.Context, tx pgx.Tx, in RelinkActivityInput) (ids.UUID, error) {
	if in.EntityType != linkEntityDeal {
		return ids.Nil, nil
	}
	project, err := inheritedProjectOf(ctx, tx, []ActivityLinkInput{{EntityType: linkEntityDeal, EntityID: in.EntityID}})
	if err != nil {
		return ids.Nil, err
	}
	return project, storekit.LockLastActivityTargets(ctx, tx, dealRelinkTargets(in.EntityID, project))
}

func dealRelinkTargets(deal, project ids.UUID) storekit.LastActivityTargets {
	return linkLockTargets([]ActivityLinkInput{{EntityType: linkEntityDeal, EntityID: deal}}, project)
}

// fileRelinkedDealProject files an activity just relinked onto a deal under
// the project lockDealRelink resolved, unless it already has a project.
func fileRelinkedDealProject(ctx context.Context, tx pgx.Tx, id ids.ActivityID, project ids.UUID) error {
	if project.IsZero() {
		return nil
	}
	if ok, err := mayAttachInherited(ctx, tx, project); err != nil || !ok {
		return err
	}
	_, _, err := relinkActivityRow(ctx, tx, id, RelinkActivityInput{
		EntityType: linkEntityProject, EntityID: project, unfiledOnly: true, evidence: inheritedProjectEvidence(),
	}, linkColumn(linkEntityProject))
	return err
}

// settleDisplaced follows a replacing relink: a moved contact takes its
// participant rows along, and a deal taken off takes the project it brought.
func settleDisplaced(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, displaced []ids.UUID) error {
	if !in.ReplaceExistingOfType || len(displaced) == 0 {
		return nil
	}
	switch in.EntityType {
	case linkEntityContact:
		return repointDisplacedParticipants(ctx, tx, id, in.EntityID, displaced)
	case linkEntityDeal:
		_, err := dropOrphanedInheritedProject(ctx, tx, id, in.EntityID)
		return err
	}
	return nil
}

// dropOrphanedInheritedProject takes off an inherited project link that no
// deal of the activity, nor the incoming one, still belongs to. A link that
// stamped retention stays for the project-filing undo to judge. It answers
// whether a link came off.
func dropOrphanedInheritedProject(ctx context.Context, tx pgx.Tx, id ids.ActivityID, incoming ids.UUID) (bool, error) {
	rows, err := tx.Query(ctx, `DELETE FROM activity_link l
		 WHERE l.activity_id = $1 AND l.entity_type = 'project'
		   AND `+provenance.InheritedProjectLinkSQL("l.activity_id", "l.project_id")+`
		   AND NOT EXISTS (SELECT 1 FROM deal d WHERE d.project_id = l.project_id AND (d.id = $2
		         OR d.id IN (SELECT dl.deal_id FROM activity_link dl WHERE dl.activity_id = $1 AND dl.entity_type = 'deal')))
		   AND NOT EXISTS (SELECT 1 FROM activity_retention_evidence e
		         WHERE e.activity_id = l.activity_id AND e.project_id = l.project_id)
		 RETURNING l.project_id`, id, incoming)
	if err != nil {
		return false, err
	}
	removed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(removed) == 0 {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", id.UUID,
		relinkBefore(linkEntityProject, removed), map[string]any{fieldEntityType: linkEntityProject}, inheritedProjectEvidence())
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: relinkedChangedFields(linkEntityProject, removed[0]),
	})
}

// answerOnDealFiling files each row a batch door moves onto a deal under the
// project the door resolved, and answers its conversation's question. The
// single door does both too. The first of a conversation's rows settles it.
func (s *Store) answerOnDealFiling(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, project ids.UUID) error {
	if in.EntityType != linkEntityDeal {
		return nil
	}
	if err := fileRelinkedDealProject(ctx, tx, id, project); err != nil {
		return err
	}
	return s.answerWhichDealTx(ctx, tx, id, in.EntityID)
}

// projectLinksFirst orders a copied link set so its own project lands before
// any deal could bring one. A set that names a project then inherits none.
func projectLinksFirst(links []ActivityLinkInput) []ActivityLinkInput {
	ordered := slices.Clone(links)
	slices.SortStableFunc(ordered, func(a, b ActivityLinkInput) int {
		switch {
		case a.EntityType == b.EntityType:
			return 0
		case a.EntityType == linkEntityProject:
			return -1
		case b.EntityType == linkEntityProject:
			return 1
		}
		return 0
	})
	return ordered
}
