// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an activity is about a deal it is linked to. A colleague or the
// classifier marks the link. A wrong link is marked, never taken off. The
// deal's timeline still shows it, and capture does not file it again.
// relstrength.DealEvidenceSQL keeps a link marked unrelated or uncertain out of
// every reading of whether the deal is alive.

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The audit keys of a deal link's mark.
const (
	fieldDealRelevance   = "deal_relevance"
	fieldRelevanceSource = "relevance_source"
)

// dealMark is one mark of a deal link, with what its audit row adds.
type dealMark struct {
	relevance, source  string
	decision, evidence map[string]any
}

// SetDealRelevance records a colleague's word that the activity is, or is not,
// about deal. The caller must be able to change both the activity and the deal.
func (s *Store) SetDealRelevance(ctx context.Context, id ids.ActivityID, deal ids.UUID, relevance string) (crmcontracts.ActivityDealRelevance, error) {
	out := crmcontracts.ActivityDealRelevance{DealId: openapi_types.UUID(deal), RelevanceSource: relstrength.RelevanceByHuman}
	if err := httperr.RequireBodyID("deal_id", deal); err != nil {
		return out, err
	}
	if relevance != relstrength.DealRelevant && relevance != relstrength.DealUnrelated {
		return out, httperr.Validation(fieldDealRelevance, "invalid", "Say relevant or unrelated.")
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return out, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var project ids.UUID
		if relevance == relstrength.DealRelevant {
			var err error
			if project, err = lockDealRelink(ctx, tx, RelinkActivityInput{EntityType: linkEntityDeal, EntityID: deal}); err != nil {
				return err
			}
		}
		if err := lockMarkedLink(ctx, tx, id, deal); err != nil {
			return err
		}
		if relevance == relstrength.DealUnrelated {
			decline, err := declineDealTx(ctx, tx, id, deal)
			if err == nil && !decline.linked {
				return apperrors.ErrNotFound
			}
			out.Changed, out.ProjectUnfiled, out.ProjectKept = decline.marked, decline.project.unfiled, decline.project.kept
			return err
		}
		changed, err := writeDealMarkTx(ctx, tx, id, deal, dealMark{relevance: relevance, source: relstrength.RelevanceByHuman})
		if err != nil || !changed {
			return err
		}
		out.Changed = true
		out.ProjectFiled, err = refileDealProject(ctx, tx, id, project)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	out.DealRelevance = crmcontracts.ActivityDealRelevanceDealRelevance(relevance)
	return out, err
}

// lockMarkedLink locks the activity for a filing change and asks that the
// deal is the caller's to change. A deal they cannot reach answers not found,
// and one that is not theirs answers forbidden.
func lockMarkedLink(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID) error {
	if err := lockForFilingChange(ctx, tx, id); err != nil {
		return err
	}
	return auth.EnsureChangeable(ctx, tx, linkEntityDeal, deal)
}

// refileDealProject files the activity on the project lockDealRelink resolved
// for the deal, unless it already has one. It answers whether it did.
func refileDealProject(ctx context.Context, tx pgx.Tx, id ids.ActivityID, project ids.UUID) (bool, error) {
	if project.IsZero() {
		return false, nil
	}
	if ok, err := mayAttachInherited(ctx, tx, project); err != nil || !ok {
		return false, err
	}
	filed, _, err := relinkActivityRow(ctx, tx, id, RelinkActivityInput{
		EntityType: linkEntityProject, EntityID: project, unfiledOnly: true, evidence: inheritedProjectEvidence(),
	}, linkColumn(linkEntityProject))
	return filed, err
}

// MarkDealLinkTx records the classifier's mark on the activity's link to deal,
// under the activity's row lock. A colleague's mark stands. It answers whether
// the link changed; a link or row that is gone answers false.
func MarkDealLinkTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, relevance string, evidence map[string]any) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	changed, err := writeDealMarkTx(ctx, tx, id, deal, dealMark{relevance: relevance, source: relstrength.RelevanceByClassifier, evidence: evidence})
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return changed, err
}

// writeDealMarkTx writes mark on the activity's link to deal, which its caller
// holds locked, with the version bump, the audit row and the relink event. A
// classifier's mark never replaces a colleague's. It answers whether the link
// changed; an activity with no link to deal answers ErrNotFound.
func writeDealMarkTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, mark dealMark) (bool, error) {
	var relevance, source *string
	err := tx.QueryRow(ctx, `SELECT deal_relevance, relevance_source FROM activity_link
		 WHERE activity_id = $1 AND entity_type = 'deal' AND deal_id = $2 FOR UPDATE`, id, deal).Scan(&relevance, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the deal link's mark: %w", err)
	}
	if mark.source == relstrength.RelevanceByClassifier && source != nil && *source == relstrength.RelevanceByHuman {
		return false, nil
	}
	if relevance != nil && source != nil && *relevance == mark.relevance && *source == mark.source {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_link SET deal_relevance = $3, relevance_source = $4
		 WHERE activity_id = $1 AND entity_type = 'deal' AND deal_id = $2`, id, deal, mark.relevance, mark.source); err != nil {
		return false, fmt.Errorf("activities: marking the deal link: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1`, id); err != nil {
		return false, err
	}
	before := map[string]any{fieldEntityType: linkEntityDeal, fieldEntityID: deal, fieldDealRelevance: relevance, fieldRelevanceSource: source}
	after := map[string]any{fieldEntityType: linkEntityDeal, fieldEntityID: deal, fieldDealRelevance: mark.relevance, fieldRelevanceSource: mark.source}
	maps.Copy(after, mark.decision)
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", id.UUID, before, after, mark.evidence)
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: relinkedChangedFields(linkEntityDeal, deal),
	})
}
