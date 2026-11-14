// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// The cg:chat-links consumer. An event on a deal, project, offer, contract or
// activity becomes a pending item of the channels linked to those records. The flush (flush.go) turns the items into one post.
// From the contact stream it takes only retention.restricted.
//
// An activity reaches a channel only when the whole workspace may read it.
// sharedActivity decides that, both when the item is recorded and again
// before the post is sent.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Sync records events for the channels of the records they touch.
type Sync struct {
	pool  *pgxpool.Pool
	db    *database.DB
	store *chatlinks.Store
}

// New builds the consumer over the chat links store. db names the
// installation's workspace, which an event does not carry.
func New(pool *pgxpool.Pool, db *database.DB, store *chatlinks.Store) *Sync {
	return &Sync{pool: pool, db: db, store: store}
}

// HandleEvent routes one envelope. An event no channel shows answers nil.
func (s *Sync) HandleEvent(ctx context.Context, env events.Envelope) error {
	if _, linked := chatRecordQueries[env.Entity.Type]; env.Entity.ID == ids.Nil || !linked && !events.NarrowsActivity(env) {
		return nil
	}
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = systemContext(ctx, ws.UUID, env)
	if events.NarrowsActivity(env) {
		if err := s.withdrawIfNarrowed(ctx, env.Entity.ID); err != nil {
			return err
		}
	}
	if env.Type == "deal.updated" && chatChangedField(env.Payload, "project_id") {
		if err := s.store.AdoptProject(ctx, env.Entity.ID); err != nil {
			return err
		}
	}
	tier, detail, posts := chatTier(env)
	if !posts {
		return nil
	}
	ev := chatlinks.Event{EventID: env.EventID, EventType: env.Type, EntityType: env.Entity.Type,
		EntityID: env.Entity.ID, Tier: tier, Detail: detail}
	if env.Entity.Type == "activity" {
		shared, err := s.activityPosts(ctx, env.Entity.ID)
		if err != nil || !shared {
			return err
		}
		ev.ActivityID = &env.Entity.ID
	}
	if ev.Deals, ev.Projects, err = s.recordsOf(ctx, env.Entity.Type, env.Entity.ID); err != nil {
		return err
	}
	return s.store.RecordEvent(ctx, ev)
}

// systemContext runs the consumer as its own system actor in the
// installation's workspace, traced to the event that caused it.
func systemContext(ctx context.Context, ws ids.UUID, env events.Envelope) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	ctx = principal.WithCausationEvent(ctx, env.EventID)
	return principal.WithActor(ctx, chatLinksPrincipal)
}

// chatLinksPrincipal is the actor of everything this engine writes.
var chatLinksPrincipal = principal.Principal{
	Type: principal.PrincipalSystem, ID: "system:chat_links",
	Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
}

// activityPosts reports whether an activity may be posted: shared with the
// whole workspace, and not one the chat itself delivered.
func (s *Sync) activityPosts(ctx context.Context, id ids.UUID) (bool, error) {
	var shared sharedText
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		shared, err = sharedActivity(ctx, tx, id)
		return err
	})
	return shared.ok && shared.sourceSystem != chatlinks.ProviderDispact, err
}

// withdrawIfNarrowed takes an activity out of the channels it reached once the
// workspace may no longer read it.
func (s *Sync) withdrawIfNarrowed(ctx context.Context, id ids.UUID) error {
	posts, err := s.activityPosts(ctx, id)
	if err != nil || posts {
		return err
	}
	return s.store.Withdraw(ctx, id)
}

// sharedText is an activity as a channel may show it.
type sharedText struct {
	ok           bool
	kind         string
	subject      string
	sourceSystem string
}

// sharedActivity is the only reader of activity text in the chat engine. It
// reads the row live under the workspace-only audience rule, so private and
// selectively shared activities, and archived or restricted ones, read as
// absent.
// Held by: TestTheChatEngineReadsActivityTextOnlyThroughSharedActivity (backend/internal/compose/chatsync/sync_test.go)
func sharedActivity(ctx context.Context, tx pgx.Tx, id ids.UUID) (sharedText, error) {
	var out sharedText
	err := tx.QueryRow(ctx, `SELECT a.kind, coalesce(a.subject, ''), coalesce(a.source_system, '')
		FROM activity a WHERE a.id = $1 AND a.archived_at IS NULL AND a.restricted_at IS NULL`+
		auth.AudienceWorkspaceOnly("a"), id).Scan(&out.kind, &out.subject, &out.sourceSystem)
	if err == pgx.ErrNoRows {
		return sharedText{}, nil
	}
	out.ok = err == nil
	return out, err
}

// recordsOf names the deals and projects an event touches. A deal also
// touches its current project, read when the event is handled.
func (s *Sync) recordsOf(ctx context.Context, entity string, id ids.UUID) ([]ids.UUID, []ids.UUID, error) {
	query, ok := chatRecordQueries[entity]
	if !ok {
		return nil, nil, nil
	}
	var deals, projects []ids.UUID
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var deal, project *ids.UUID
			if err := rows.Scan(&deal, &project); err != nil {
				return err
			}
			if deal != nil {
				deals = append(deals, *deal)
			}
			if project != nil {
				projects = append(projects, *project)
			}
		}
		return rows.Err()
	})
	return deals, projects, err
}

// chatRecordQueries read, per entity, the (deal, project) pairs it touches.
var chatRecordQueries = map[string]string{
	"deal":     `SELECT id, project_id FROM deal WHERE id = $1`,
	"project":  `SELECT NULL::uuid, id FROM project WHERE id = $1`,
	"offer":    `SELECT d.id, d.project_id FROM offer o JOIN deal d ON d.id = o.deal_id WHERE o.id = $1`,
	"contract": `SELECT c.deal_id, coalesce(c.project_id, d.project_id) FROM contract c LEFT JOIN deal d ON d.id = c.deal_id WHERE c.id = $1`,
	"activity": `SELECT l.deal_id, coalesce(l.project_id, d.project_id) FROM activity_link l
		LEFT JOIN deal d ON d.id = l.deal_id
		WHERE l.activity_id = $1 AND (l.deal_id IS NOT NULL OR l.project_id IS NOT NULL)`,
}

// chatTier decides whether an event posts and at which tier, and keeps the
// facts its line needs. An event type not named here posts at the everything
// tier when it belongs to a channel's records, and never when it is in
// chatIgnoredEvents.
func chatTier(env events.Envelope) (string, map[string]any, bool) {
	if chatIgnoredEvents[env.Type] {
		return "", nil, false
	}
	if tier, ok := chatMilestones[env.Type]; ok {
		return tier, chatDetail(env), true
	}
	switch env.Type {
	case "deal.updated", "project.updated":
		fields := chatChangedFields(env.Payload)
		if hasAny(fields, chatImportantFields[env.Type]) {
			return chatlinks.TierImportant, map[string]any{"fields": fields}, true
		}
		return chatlinks.TierEverything, map[string]any{"fields": fields}, true
	case "activity.captured":
		var captured crmcontracts.PublicEventActivityCaptured
		if err := json.Unmarshal(env.Payload, &captured); err != nil {
			return "", nil, false
		}
		if chatImportantKinds[captured.Kind] {
			return chatlinks.TierImportant, nil, true
		}
		return chatlinks.TierEverything, nil, true
	case "activity.updated":
		fields := chatChangedFields(env.Payload)
		if fields["meeting_status"] == "held" || fields["is_done"] == true {
			return chatlinks.TierImportant, map[string]any{"fields": fields}, true
		}
		return "", nil, false
	}
	if _, ok := chatRecordQueries[strings.SplitN(env.Type, ".", 2)[0]]; ok {
		return chatlinks.TierEverything, nil, true
	}
	return "", nil, false
}

// chatMilestones are the steps every level but silent posts.
var chatMilestones = map[string]string{
	"deal.stage_changed":      chatlinks.TierMilestone,
	"deal.archived":           chatlinks.TierMilestone,
	"deal.restored":           chatlinks.TierMilestone,
	"offer.sent":              chatlinks.TierMilestone,
	"offer.accepted":          chatlinks.TierMilestone,
	"offer.rejected":          chatlinks.TierMilestone,
	"contract.created":        chatlinks.TierMilestone,
	"contract.status_changed": chatlinks.TierMilestone,
	"contract.archived":       chatlinks.TierMilestone,
	"project.phase_changed":   chatlinks.TierMilestone,
	"deal.owner_changed":      chatlinks.TierImportant,
	"offer.created":           chatlinks.TierImportant,
}

// chatIgnoredEvents never post: they are bookkeeping, erasure, or happen
// before a record could have a channel.
var chatIgnoredEvents = map[string]bool{
	"deal.created":                     true,
	"deal.merged":                      true,
	"deal.erased":                      true,
	"project.created":                  true,
	"project.health_assessed":          true,
	"activity.archived":                true,
	"activity.disposition_recorded":    true,
	"activity.marked_handled":          true,
	"activity.link_suggestion_changed": true,
	"activity.readiness_recorded":      true,
	"activity.invitation_answered":     true,
	"offer.superseded":                 true,
}

// chatImportantFields are the fields whose change a team acts on.
var chatImportantFields = map[string][]string{
	"deal.updated":    {"amount_minor", "currency", "expected_close_date", "name"},
	"project.updated": {"name", "owner_id", "target_end_date"},
}

// chatImportantKinds are the activity kinds the important level posts.
var chatImportantKinds = map[string]bool{"note": true, "meeting": true, "email": true}

// chatDetail keeps the one fact a milestone line names.
func chatDetail(env events.Envelope) map[string]any {
	var payload map[string]any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return nil
	}
	detail := map[string]any{}
	for _, key := range []string{"to_stage_name", "to_status", "to_phase", "status"} {
		if value, ok := payload[key].(string); ok && value != "" {
			detail[key] = value
		}
	}
	return detail
}

func chatChangedFields(payload json.RawMessage) map[string]any {
	var body struct {
		ChangedFields map[string]any `json:"changed_fields"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil
	}
	return body.ChangedFields
}

func chatChangedField(payload json.RawMessage, name string) bool {
	_, ok := chatChangedFields(payload)[name]
	return ok
}

func hasAny(fields map[string]any, names []string) bool {
	for _, name := range names {
		if _, ok := fields[name]; ok {
			return true
		}
	}
	return false
}
