// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// chatLinkEntity is the audit entity of a link.
const chatLinkEntity = "chat_link"

// insertLink writes the link under the record's lock, with its audit row and
// the flush that posts the card. A link that asks for the talk so far queues
// its bot's pull too, rather than waiting for the sweep. A deal whose project has no channel brings
// the project along, because a deal and its project share one channel.
func (s *Store) insertLink(ctx context.Context, kind string, id ids.UUID, session Session, channel chatwire.Channel, req LinkRequest) (Link, error) {
	actor, err := linkActor(ctx)
	if err != nil {
		return Link{}, err
	}
	var out Link
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, kind, id); err != nil {
			return err
		}
		ctx, err = s.holdLinkParticipant(ctx, tx, kind, actor, req)
		if err != nil {
			return err
		}

		if err := requireLiveConnection(ctx, tx, session.ConnectionID); err != nil {
			return err
		}
		var company *ids.UUID
		if kind == RecordCompany {
			company = &id
		}
		deal, project, err := linkedRecords(ctx, tx, kind, id)
		if err != nil {
			return err
		}

		if err := holdChannelAssociation(ctx, tx, session.ConnectionID, channel.Slug, req.RootID); err != nil {
			return err
		}
		_, nativeIdentity := session.Chat.(chatwire.RootReader)
		includeHistory := req.SummarizeHistory != nil && *req.SummarizeHistory
		args := []any{
			session.ConnectionID, req.Provider, channel.Slug, channel.Name, deal, project,
			req.Verbosity, includeHistory, actor, company, MessageDetailBrief, optionalRoot(req.RootID), includeHistory, channel.PostingAllowed == nil || *channel.PostingAllowed, true, nativeIdentity,
		}
		holders := make([]string, len(args))
		for i := range args {
			holders[i] = fmt.Sprintf("$%d", i+1)
		}
		out, err = scanLink(tx.QueryRow(ctx, `INSERT INTO chat_link
            (connection_id, provider, channel_slug, channel_name, deal_id, project_id,
             verbosity, backfill_requested, linked_by, company_id,message_detail,root_id,include_history,post_supported,post_disabled,native_identity)
            VALUES (`+strings.Join(holders, ",")+`) RETURNING `+linkColumns, args...))
		if err != nil {
			return linkUniqueness(err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", chatLinkEntity, out.ID, nil, auditImage(out)); err != nil {
			return err
		}
		if out.BackfillRequested {
			if err := s.jobs.EnqueuePullTx(ctx, tx, out.ConnectionID); err != nil {
				return err
			}
		}
		return s.jobs.EnqueueFlushTx(ctx, tx, out.ID)
	})
	return out, err
}

// linkedRecords names the records a new link carries. That is the record, and
// for a deal also its project when the project has no channel of its own.
func linkedRecords(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (*ids.UUID, *ids.UUID, error) {
	if kind == RecordCompany {
		return nil, nil, nil
	}
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
	return s.SetSyncOptions(ctx, kind, id, SyncOptions{Verbosity: &verbosity})
}

// SyncOptions keeps incoming capture and outgoing delivery choices independent.
type SyncOptions struct {
	ExpectedLink    *ids.UUID
	ExpectedVersion *int64
	Verbosity       *string
	Read, Post      *bool
	MessageDetail   *string
}

// SetSyncOptions changes read and post directions under current record authority.
func (s *Store) SetSyncOptions(ctx context.Context, kind string, id ids.UUID, options SyncOptions) (State, error) {
	if err := options.validate(); err != nil {
		return State{}, err
	}
	var out State
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.FilingContextTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordUpdate(ctx, kind); err != nil {
			return err
		}
		if err := auth.HoldWritableLive(ctx, tx, kind, id); err != nil {
			return err
		}
		before, err := liveLinkOf(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		if err := options.validateTarget(before); err != nil {
			return err
		}
		after, err := applySyncOptionsTx(ctx, tx, kind, id, before, options)
		if err != nil {
			return err
		}
		if err := s.queueSyncChanges(ctx, tx, before, after); err != nil {
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
	if err := requireRecordUpdate(ctx, kind); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureRetractable(ctx, tx, kind, id); err != nil {
			return err
		}
		before, err := heldLiveLinkOf(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		return retireLink(ctx, tx, before)
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

func holdChannelAssociation(ctx context.Context, tx pgx.Tx, connection ids.UUID, channel, root string) error {
	key := connection.String() + ":" + channel
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: key}); err != nil {
		return err
	}
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_link WHERE connection_id=@connection AND channel_slug=@channel AND archived_at IS NULL AND (coalesce(root_id,'')='' OR @root=''))`, pgx.StrictNamedArgs{connectionKey: connection, channelKey: channel, rootKey: root}).Scan(&held)
	if err != nil {
		return err
	}
	if held {
		return ErrChannelAlreadyLinked
	}
	return nil
}

func (options SyncOptions) validate() error {
	if options.Verbosity != nil {
		if err := validVerbosity(*options.Verbosity); err != nil {
			return err
		}
	}
	if options.MessageDetail != nil && *options.MessageDetail != MessageDetailBrief && *options.MessageDetail != MessageDetailFull {
		return apperrors.ErrInvalidArgument
	}
	if options.Verbosity == nil && options.Read == nil && options.Post == nil && options.MessageDetail == nil {
		return apperrors.ErrInvalidArgument
	}
	return nil
}

func (s *Store) queueSyncChanges(ctx context.Context, tx pgx.Tx, before, after Link) error {
	if before.ReadDisabled && !after.ReadDisabled {
		if err := s.jobs.EnqueuePullTx(ctx, tx, after.ConnectionID); err != nil {
			return err
		}
	}
	if before.PostDisabled && !after.PostDisabled {
		if err := s.jobs.EnqueueFlushTx(ctx, tx, after.ID); err != nil {
			return err
		}
	}
	return nil
}

func (options SyncOptions) validateTarget(before Link) error {
	if options.ExpectedLink != nil && *options.ExpectedLink != before.ID || options.ExpectedVersion != nil && *options.ExpectedVersion != before.Version {
		return apperrors.ErrConflict
	}
	if options.Post != nil && *options.Post && !before.PostSupported {
		return apperrors.ErrInvalidArgument
	}
	return nil
}

func (s *Store) holdLinkParticipant(ctx context.Context, tx pgx.Tx, kind string, actor ids.UUID, req LinkRequest) (context.Context, error) {
	if req.participant == nil {
		return ctx, nil
	}
	var err error
	ctx, err = s.holdMemberTx(ctx, tx)
	if err != nil {
		return ctx, err
	}
	var consent ids.UUID
	grant := req.participant
	if err := tx.QueryRow(ctx, `SELECT id FROM chat_member_grant WHERE id=@grant AND user_id=@user AND provider=@provider AND consent_version=@version AND status='connected' FOR SHARE`, pgx.StrictNamedArgs{grantKey: grant.ID, userKey: actor, providerKey: req.Provider, versionKey: grant.Version}).Scan(&consent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ctx, messaging.ErrUnauthorized
		}
		return ctx, err
	}
	if err := requireRecordUpdate(ctx, kind); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func prepareSyncPatch(ctx context.Context, tx pgx.Tx, before Link, options SyncOptions) (*storekit.Patch, error) {
	readDisabled, postDisabled := before.ReadDisabled, before.PostDisabled
	if options.Read != nil {
		readDisabled = !*options.Read
	}
	if options.Post != nil {
		postDisabled = !*options.Post
	}

	patch := storekit.NewPatch()
	if options.Verbosity != nil {
		patch.Set("verbosity", before.Verbosity, *options.Verbosity)
	}
	detail := before.MessageDetail
	if options.MessageDetail != nil {
		detail = *options.MessageDetail
	}
	if before.storedMessageDetail == nil || options.MessageDetail != nil {
		patch.Set("message_detail", before.storedMessageDetail, &detail)
	}
	patch.Set("read_disabled", before.ReadDisabled, readDisabled)
	patch.Set("post_disabled", before.PostDisabled, postDisabled)
	if before.ReadDisabled && !readDisabled {
		var since time.Time
		if err := tx.QueryRow(ctx, "SELECT now()").Scan(&since); err != nil {
			return nil, err
		}
		patch.Set("read_since", before.ReadSince, &since)
	}
	if readDisabled {
		patch.Set("backfill_requested", before.BackfillRequested, false)
	}
	return patch, nil
}

func applySyncOptionsTx(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, before Link, options SyncOptions) (Link, error) {
	patch, err := prepareSyncPatch(ctx, tx, before, options)
	if err != nil {
		return Link{}, err
	}
	if !patch.Empty() {
		if err := patch.ApplyWithVersion(ctx, tx, "chat_link", before.ID, before.Version); err != nil {
			return Link{}, err
		}
	}
	after, err := liveLinkOf(ctx, tx, kind, id)
	if err != nil {
		return Link{}, err
	}
	if _, err := storekit.Audit(ctx, tx, "update", chatLinkEntity, after.ID, auditImage(before), auditImage(after)); err != nil {
		return Link{}, err
	}
	if !before.PostDisabled && after.PostDisabled {
		if _, err := tx.Exec(ctx, `UPDATE chat_post_item SET withdrawn_at=now()
                WHERE link_id=$1 AND posted_at IS NULL AND withdrawn_at IS NULL`, after.ID); err != nil {
			return Link{}, err
		}
	}
	return after, nil
}
