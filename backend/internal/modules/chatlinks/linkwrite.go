// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// chatLinkEntity is the audit entity of a link.
const chatLinkEntity = "chat_link"

// insertLink writes the link under the record's lock, with its audit row and
// the flush that posts the card. A deal whose project has no channel brings
// the project along, because a deal and its project share one channel.
func (s *Store) insertLink(ctx context.Context, kind string, id ids.UUID, session Session, channel dispact.Channel, req LinkRequest) (Link, error) {
	actor, err := linkActor(ctx)
	if err != nil {
		return Link{}, err
	}
	var out Link
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, kind, id); err != nil {
			return err
		}
		if err := requireLiveConnection(ctx, tx, session.ConnectionID); err != nil {
			return err
		}
		deal, project, err := linkedRecords(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		var key *string
		if req.Mode == ModeCreate {
			k := externalKey(kind, id)
			key = &k
		}
		out, err = scanLink(tx.QueryRow(ctx, `INSERT INTO chat_link
			(connection_id, provider, channel_slug, channel_name, channel_external_key, deal_id, project_id,
			 verbosity, backfill_requested, linked_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING `+linkColumns,
			session.ConnectionID, ProviderDispact, channel.Slug, channel.Name, key, deal, project,
			req.Verbosity, req.SummarizeHistory != nil && *req.SummarizeHistory, actor))
		if err != nil {
			return linkUniqueness(err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", chatLinkEntity, out.ID, nil, auditImage(out)); err != nil {
			return err
		}
		return s.flush.EnqueueFlushTx(ctx, tx, out.ID)
	})
	return out, err
}

// linkedRecords names the records a new link carries. That is the record, and
// for a deal also its project when the project has no channel of its own.
func linkedRecords(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (*ids.UUID, *ids.UUID, error) {
	if kind == RecordProject {
		return nil, &id, nil
	}
	var project *ids.UUID
	err := tx.QueryRow(ctx, `SELECT d.project_id FROM deal d
		WHERE d.id = $1 AND NOT EXISTS (
			SELECT 1 FROM chat_link l WHERE l.project_id = d.project_id AND l.archived_at IS NULL)`, id).Scan(&project)
	if errors.Is(err, pgx.ErrNoRows) || project == nil {
		return &id, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	// The project joins the channel only when the member may edit it, the
	// same authority attaching a deal to a project asks for.
	err = auth.Require(ctx, RecordProject, principal.ActionUpdate)
	if err == nil {
		err = auth.EnsureWritableLive(ctx, tx, RecordProject, *project)
	}
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return &id, nil, nil
		}
		return nil, nil, err
	}
	return &id, project, nil
}

// requireLiveConnection refuses a link whose bot was disconnected while the
// chat was being asked.
func requireLiveConnection(ctx context.Context, tx pgx.Tx, connection ids.UUID) error {
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channel_connection
		WHERE id = $1 AND status = 'connected' AND archived_at IS NULL)`, connection).Scan(&live); err != nil {
		return err
	}
	if !live {
		return ErrBotNotConnected
	}
	return nil
}

// linkUniqueness answers a lost race for one of the live-link indexes by the
// rule it broke.
func linkUniqueness(err error) error {
	constraint, unique := storekit.UniqueViolation(err)
	if !unique {
		return err
	}
	if constraint == "uq_chat_link_channel" {
		return ErrChannelAlreadyLinked
	}
	return ErrRecordAlreadyLinked
}

// SetVerbosity changes how much Margince posts into the record's channel.
func (s *Store) SetVerbosity(ctx context.Context, kind string, id ids.UUID, verbosity string) (State, error) {
	if err := auth.Require(ctx, kind, principal.ActionUpdate); err != nil {
		return State{}, err
	}
	if err := validVerbosity(verbosity); err != nil {
		return State{}, err
	}
	var out State
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, kind, id); err != nil {
			return err
		}
		before, err := liveLinkOf(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		after, err := scanLink(tx.QueryRow(ctx, `UPDATE chat_link SET verbosity = $2
			WHERE id = $1 AND version = $3 RETURNING `+linkColumns, before.ID, verbosity, before.Version))
		if err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", chatLinkEntity, after.ID, auditImage(before), auditImage(after)); err != nil {
			return err
		}
		out, err = stateOf(ctx, tx, kind, id)
		return err
	})
	return out, err
}

// Unlink stops syncing the record with its channel. The channel and the notes
// already filed stay; the link is archived so its history stays auditable.
func (s *Store) Unlink(ctx context.Context, kind string, id ids.UUID) error {
	if err := auth.Require(ctx, kind, principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureRetractable(ctx, tx, kind, id); err != nil {
			return err
		}
		before, err := liveLinkOf(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE chat_link SET archived_at = now()
			WHERE id = $1 AND archived_at IS NULL`, before.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("chatlinks: unlinking %s: %w", before.ID, ErrLinkNotFound)
		}
		_, err = storekit.Audit(ctx, tx, "archive", chatLinkEntity, before.ID, auditImage(before), nil)
		return err
	})
}

// AdoptProject brings a deal's new project into the deal's channel, because
// a deal and its project share one channel. A project with a channel of its
// own keeps it.
func (s *Store) AdoptProject(ctx context.Context, dealID ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := liveLinkOf(ctx, tx, RecordDeal, dealID)
		if errors.Is(err, ErrLinkNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		after, err := scanLink(tx.QueryRow(ctx, `WITH next AS (SELECT project_id AS next_project FROM deal WHERE id = $2)
			UPDATE chat_link SET project_id = next.next_project FROM next
			WHERE id = $1 AND next.next_project IS NOT NULL
			  AND project_id IS DISTINCT FROM next.next_project
			  AND NOT EXISTS (SELECT 1 FROM chat_link o WHERE o.project_id = next.next_project AND o.archived_at IS NULL)
			RETURNING `+linkColumns, before.ID, dealID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", chatLinkEntity, after.ID, auditImage(before), auditImage(after))
		return err
	})
}
