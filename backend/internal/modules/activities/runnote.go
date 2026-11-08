// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scheduledRunNoteLanding keeps a scheduled run to one note per deal and
// member a day. A spec runs once a day per seat, so that is one per run. It
// drops from in the deal links that already carry such a note. When it drops
// every deal link, it answers that earlier note and nothing is written. The
// member is the one the audit row names, so a new passport is no way around it.
func scheduledRunNoteLanding(ctx context.Context, tx pgx.Tx, in *LogActivityInput) (crmcontracts.Activity, bool, error) {
	actor, ok := principal.Actor(ctx)
	if in.Origin != OriginAgent || in.Kind != string(crmcontracts.ActivityKindNote) || !ok || actor.UserID == ids.Nil {
		return crmcontracts.Activity{}, false, nil
	}
	asOf := orWallClock(in.recordedAt)
	// The limit counts UTC days, so its window does not move with a zone setting.
	day := storekit.WorkspaceDay(asOf, time.UTC)
	if err := lockRunNoteDeals(ctx, tx, actor.UserID, day, in.Links); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	var kept []ActivityLinkInput
	var first *ids.ActivityID
	for _, link := range in.Links {
		if link.EntityType != linkEntityDeal {
			kept = append(kept, link)
			continue
		}
		prior, found, err := runNoteOnDeal(ctx, tx, actor.UserID, link.EntityID, day)
		if err != nil {
			return crmcontracts.Activity{}, false, err
		}
		if !found {
			kept = append(kept, link)
		} else if first == nil {
			first = &prior
		}
	}
	in.Links = kept
	if first == nil || slices.ContainsFunc(kept, func(l ActivityLinkInput) bool { return l.EntityType == linkEntityDeal }) {
		return crmcontracts.Activity{}, false, nil
	}
	out, err := readActivityAt(ctx, tx, *first, storekit.LiveOnly, asOf)
	return out, err == nil, err
}

// lockRunNoteDeals takes one transaction lock per member, deal and day, in deal
// order. Two writes of one run then cannot both find no note and both write.
func lockRunNoteDeals(ctx context.Context, tx pgx.Tx, member ids.UUID, day time.Time, links []ActivityLinkInput) error {
	var deals []string
	for _, link := range links {
		if link.EntityType == linkEntityDeal {
			deals = append(deals, link.EntityID.String())
		}
	}
	slices.Sort(deals)
	for _, deal := range slices.Compact(deals) {
		key := "scheduled_run_note:" + member.String() + ":" + deal + ":" + day.Format(time.DateOnly)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
			return fmt.Errorf("activities: locking a scheduled run's note on a deal: %w", err)
		}
	}
	return nil
}

// runNoteOnDeal finds the note a scheduled run wrote on deal for member since
// day began.
func runNoteOnDeal(ctx context.Context, tx pgx.Tx, member, deal ids.UUID, day time.Time) (ids.ActivityID, bool, error) {
	var prior ids.ActivityID
	err := tx.QueryRow(ctx, `
		SELECT a.id FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		 WHERE a.kind = $2 AND a.origin = $3 AND a.archived_at IS NULL AND a.created_at >= $4
		   AND EXISTS (SELECT 1 FROM audit_log al
		                WHERE al.entity_type = 'activity' AND al.entity_id = a.id
		                  AND al.action = 'create' AND al.on_behalf_of = $5)
		 ORDER BY a.created_at, a.id LIMIT 1`,
		deal, string(crmcontracts.ActivityKindNote), OriginAgent, day, member).Scan(&prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return prior, false, nil
	}
	if err != nil {
		return prior, false, fmt.Errorf("activities: reading today's scheduled-run note on a deal: %w", err)
	}
	return prior, true, nil
}

// refuseAnotherMembersDeal refuses a scheduled run filing an activity under a
// deal its member does not own. The owner is read under a share lock in the
// write's transaction, so a deal reassigned after the run listed it is refused.
func refuseAnotherMembersDeal(ctx context.Context, tx pgx.Tx, deal ids.UUID) error {
	if _, scheduled := principal.AgentRunID(ctx); !scheduled {
		return nil
	}
	var owner *ids.UUID
	err := tx.QueryRow(ctx, `SELECT owner_id FROM deal WHERE id = $1 FOR SHARE`, deal).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("activities: reading the owner of a deal a scheduled run files under: %w", err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok || owner == nil || *owner != actor.UserID {
		return &ScheduledRunForeignDealError{}
	}
	return nil
}

// ScheduledRunForeignDealError refuses a scheduled run filing under a deal its
// member does not own. Each member's run covers only that member's deals.
type ScheduledRunForeignDealError struct{}

func (e *ScheduledRunForeignDealError) Error() string {
	return "a scheduled run files only under the deals its member owns, and this deal belongs to somebody else"
}

// FieldFault names the links array, where the deal to drop is.
func (e *ScheduledRunForeignDealError) FieldFault() (field, code, message string) {
	return fieldLinks, "deal_not_owned_by_member", e.Error() + ". Leave this deal out; its owner's own run covers it"
}
