// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A card attached to captured mail imports itself.
//
// Handing over a .vcf by mail is the same act as handing one over on paper, and
// until this consumer existed only the paper half worked: the manual upload
// parsed a card, while a card that arrived as an attachment sat in the object
// store as an opaque blob forever. The contact who mailed their details got
// nothing, which is the shape of failure nobody reports — the record simply
// stays stale.
//
// A SECOND TRIGGER rather than a branch inside the signature one, and the
// difference is what each needs to run. A signature is read by a model, so its
// trigger is worth nothing where no model is configured. A card is PARSED —
// deterministic text, no inference, no token budget — so gating it on the same
// brain would delete the feature in an AI-less deployment for a reason that
// does not apply to it.
//
// THE TRIGGER IS THE EVENT, NOT THE WRITER: activity.captured reaches the
// outbox because the write shape puts it there, so every connector lands here
// without knowing this consumer exists.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// vcardIngestFreshWindow is how old a capture event may be and still queue an
// import.
//
// The bound is what makes first deployment safe: a new consumer group starts at
// stream position 0, so the first boot replays the whole activity stream into
// this handler. Without it, every card ever mailed would import at once — and
// unlike the signature pass, which reconciles nightly, THIS has no second
// chance to correct itself, because each import writes people. An hour is
// generous for a live event's delivery lag while excluding replayed backlog.
const vcardIngestFreshWindow = time.Hour

// VCardIngestTrigger imports the cards attached to one captured email.
type VCardIngestTrigger struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	log     *slog.Logger
}

// NewVCardIngestTrigger builds the trigger over an insert-only jobs runner.
func NewVCardIngestTrigger(pool *pgxpool.Pool, enqueue *jobs.Runner, log *slog.Logger) *VCardIngestTrigger {
	return &VCardIngestTrigger{pool: pool, enqueue: enqueue, log: log}
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic.
func (t *VCardIngestTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != "activity.captured" {
		return nil
	}
	var payload crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		// A payload this consumer cannot read is not a reason to wedge the
		// group. Nothing else imports mailed cards, so this one is lost — said
		// out loud rather than swallowed, because there is no reconciler behind
		// this consumer to find it later.
		t.log.WarnContext(ctx, "vcard ingest trigger: unreadable capture payload",
			"event", env.EventID.String(), "err", err)
		return nil
	}
	// The contract's own spelling of the kind, since this is reading a contract
	// payload: a literal here and a renamed enum there would compile and match
	// nothing.
	if payload.Kind != string(crmcontracts.ActivityKindEmail) {
		return nil
	}
	if time.Since(env.OccurredAt) > vcardIngestFreshWindow {
		return nil
	}
	// The envelope names the subject; the payload describes it. The activity
	// id is not repeated in the payload, so the entity ref is where it lives.
	activityID := env.Entity.ID
	// The probe before the enqueue: almost no captured mail carries a card, and
	// a job per message would put the whole mail volume through the queue to
	// discover that. One indexed read answers it here.
	carries, err := t.carriesCard(ctx, activityID)
	if err != nil {
		return err
	}
	if !carries {
		return nil
	}
	ws, err := InstallationDB(t.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	// ByArgs alone, over the active states: the args name ONE message, so two
	// deliveries of the same capture event collapse onto one import while two
	// different messages each get their own. This is the opposite choice from
	// the signature trigger, which collapses a whole burst onto one workspace
	// pass — that one re-derives its own candidates, and this one cannot,
	// because the message it names is the only place its cards live.
	child := VCardIngestArgs{Workspace: ws.UUID, Activity: activityID}
	return t.enqueue.Enqueue(ctx, child, vcardIngestInsertOpts())
}

// carriesCard answers whether this message has a live attachment that could be
// a vCard.
//
// Content type OR filename, because neither alone is sound. A sending client
// picks the type, and Gmail delivers a hand-attached .vcf as text/plain — the
// message this feature was built against did exactly that, so a content-type
// test alone would have found nothing. The other way round fails too: a card
// generated by a CRM export is often named `contact` with no extension while
// carrying the correct type. Matching either is what makes the probe find a
// card that any real client sent.
//
// This is a PROBE and not the decision. It reads the attachment metadata to
// decide whether the message is worth a job; the job re-reads under a lock and
// parses the bytes, which is what actually establishes a card is present.
//
// Keyed on (entity_type, entity_id) rather than the activity_id column beside
// them, because that pair is what idx_attachment_entity indexes and activity_id
// is indexed by nothing. The two agree — capture writes both — but only one of
// them keeps this off a sequential scan of a table that grows forever, and a
// probe that costs a full scan per captured message is worse than the job it
// was added to avoid.
func (t *VCardIngestTrigger) carriesCard(ctx context.Context, activity ids.UUID) (bool, error) {
	var found bool
	err := t.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM attachment
			 WHERE entity_type = 'activity' AND entity_id = $1
			   AND archived_at IS NULL
			   AND (lower(content_type) IN ('text/vcard', 'text/x-vcard', 'text/directory')
			        OR lower(filename) LIKE '%.vcf')
		)`, activity).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("compose: probing a captured message for a card: %w", err)
	}
	return found, nil
}
