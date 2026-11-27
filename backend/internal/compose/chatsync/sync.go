// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// The cg:chat-links consumer. An event on a deal, project, offer, contract or
// activity becomes a pending item of the channels linked to those records. The flush (flush.go) turns the items into one post.
// From the contact stream it takes retention.restricted and retention.applied.
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
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	if _, linked := chatRecordQueries[env.Entity.Type]; env.Entity.ID == ids.Nil || !linked && env.Entity.Type != chatlinks.RecordCompany && !events.NarrowsActivity(env) {
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
	if chatReleasingEvents[env.Type] && (env.Entity.Type == chatlinks.RecordDeal || env.Entity.Type == chatlinks.RecordProject || env.Entity.Type == chatlinks.RecordCompany) {
		if err := s.store.ReleaseArchived(ctx, env.Entity.Type, env.Entity.ID); err != nil {
			return err
		}
	}
	if env.Type == eventDealUpdated && chatChangedField(env.Payload, "project_id") {
		if err := s.store.AdoptProject(ctx, env.Entity.ID); err != nil {
			return err
		}
	}
	tier, detail, posts := chatTier(env)
	if !posts {
		return nil
	}
	ev := chatlinks.Event{EventID: env.EventID, EventType: env.Type, Tier: tier, Detail: detail}
	if env.Entity.Type == "activity" {
		shared, err := s.sharedActivityText(ctx, env.Entity.ID)
		if err != nil || !shared.ok || chatlinks.IsChatProvider(shared.sourceSystem) {
			return err
		}
		ev.ActivityID = &env.Entity.ID
		if chatChangedField(env.Payload, "relinked") {
			ev.Tier = activityTier(shared.kind)
		}
	}
	if ev.Deals, ev.Projects, err = s.recordsOf(ctx, env.Entity.Type, env.Entity.ID); err != nil {
		return err
	}
	if env.Entity.Type == chatlinks.RecordCompany {
		ev.Companies = []ids.UUID{env.Entity.ID}
	}
	if env.Entity.Type == "activity" {
		err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT DISTINCT company_id FROM activity_link WHERE activity_id=$1 AND company_id IS NOT NULL`, env.Entity.ID)
			if err != nil {
				return err
			}
			ev.Companies, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
			return err
		})
		if err != nil {
			return err
		}
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
	shared, err := s.sharedActivityText(ctx, id)
	return shared.ok && !chatlinks.IsChatProvider(shared.sourceSystem), err
}

// sharedActivityText reads the activity as a channel may show it.
func (s *Sync) sharedActivityText(ctx context.Context, id ids.UUID) (sharedText, error) {
	var shared sharedText
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		shared, err = sharedActivity(ctx, tx, id)
		return err
	})
	return shared, err
}

// activityTier is the tier an activity posts at, by its kind.
func activityTier(kind string) string {
	if chatImportantKinds[kind] {
		return chatlinks.TierImportant
	}
	return chatlinks.TierEverything
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

// sharedText is an activity as a channel may show it. body is read only by
// sharedNote.
type sharedText struct {
	complete     bool
	ok           bool
	kind         string
	subject      string
	body         string
	sourceSystem string
}

// sharedActivity is an activity as a channel may show it, by id.
func sharedActivity(ctx context.Context, tx pgx.Tx, id ids.UUID) (sharedText, error) {
	return sharedRow(ctx, tx, "a.id = $1", "a.occurred_at DESC, a.id DESC", id)
}

// sharedRow is the only reader of activity text in the chat engine. It reads
// the first row that where selects in order, under the workspace-only audience rule.
// Private, selectively shared, archived and restricted activities read as absent.
// Held by: TestTheChatEngineReadsActivityTextOnlyThroughSharedRow (backend/internal/compose/chatsync/sync_test.go)
func sharedRow(ctx context.Context, tx pgx.Tx, where, order string, args ...any) (sharedText, error) {
	var out sharedText
	err := tx.QueryRow(ctx, `SELECT a.kind, coalesce(a.subject, ''), coalesce(a.body, ''), coalesce(a.source_system, ''), `+provenance.MailBodyCompleteSQL("a")+`
		FROM activity a WHERE a.archived_at IS NULL AND a.restricted_at IS NULL AND `+where+
		auth.AudienceWorkspaceOnly("a")+` ORDER BY `+order+` LIMIT 1`, args...).
		Scan(&out.kind, &out.subject, &out.body, &out.sourceSystem, &out.complete)
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
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
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
	if (env.Type == "offer.updated" && crmcontracts.OfferSourceOnly(env.Payload)) || chatIgnoredEvents[env.Type] {
		return "", nil, false
	}
	if tier, ok := chatMilestones[env.Type]; ok {
		return tier, chatDetail(env), true
	}
	switch env.Type {
	case eventDealUpdated, eventProjectUpdated:
		fields := chatChangedFields(env.Payload)
		if hasAny(fields, chatImportantFields[env.Type]) {
			return chatlinks.TierImportant, map[string]any{detailFields: fields}, true
		}
		return chatlinks.TierEverything, map[string]any{detailFields: fields}, true
	case "activity.captured":
		var captured crmcontracts.PublicEventActivityCaptured
		if err := json.Unmarshal(env.Payload, &captured); err != nil {
			return "", nil, false
		}
		return activityTier(captured.Kind), nil, true
	case "activity.updated":
		fields := chatChangedFields(env.Payload)
		// A newly filed activity reaches the channel of the record it was filed
		// on, at the tier its kind earns. HandleEvent reads the kind.
		if _, relinked := fields["relinked"]; relinked {
			return chatlinks.TierEverything, nil, true
		}
		if fields["meeting_status"] == "held" || fields["is_done"] == true {
			return chatlinks.TierImportant, map[string]any{detailFields: fields}, true
		}
		return "", nil, false
	}
	if _, ok := chatRecordQueries[strings.SplitN(env.Type, ".", 2)[0]]; ok {
		return chatlinks.TierEverything, nil, true
	}
	return "", nil, false
}

// The event types and changed fields the tiers and the lines both name.
const (
	eventDealUpdated     = "deal.updated"
	eventDealArchived    = "deal.archived"
	eventProjectArchived = "project.archived"
	eventProjectUpdated  = "project.updated"
	detailFields         = "fields"
	fieldCurrency        = "currency"
	fieldName            = "name"
)

// chatReleasingEvents may leave a deal or project archived. A retention
// policy archives a deal under retention.applied, not deal.archived.
var chatReleasingEvents = map[string]bool{"company.archived": true, eventDealArchived: true, eventProjectArchived: true, "retention.applied": true}

// chatMilestones are the steps every level but silent posts.
var chatMilestones = map[string]string{
	"deal.stage_changed":      chatlinks.TierMilestone,
	eventDealArchived:         chatlinks.TierMilestone,
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
	eventDealUpdated:    {"amount_minor", fieldCurrency, "expected_close_date", fieldName},
	eventProjectUpdated: {fieldName, "owner_id", "target_end_date"},
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
