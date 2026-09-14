// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The consumer that records what the installation owes a new contact.
//
// contacts writes WHY a contact exists; consent decides what that obliges and
// holds the queue of duties owed. Neither imports the other, so the edge is
// injected here.
//
// It reads the acquisition rather than taking it from the event payload.
// contact.created carries only full_name — widening a shipped public event to
// carry an acquisition id would change a contract no external consumer asked
// for. The acquisition row is written in the SAME transaction as the contact and
// the event (contacts/resolvecreate.go), so by the time this runs it is there.
//
// IDEMPOTENCY IS THE UNIQUE INDEX, not this handler and not the dedupe cache.
// privacy_notice_case carries one row per acquisition_id and the writer says
// ON CONFLICT DO NOTHING, so a redelivered contact.created finds the duty
// already recorded and does nothing. events.Dedupe sits in front of that as a
// cache, never as the guarantee — it marks AFTER the effect, so a crash in that
// window replays, and replay has to be free.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// noticeCaseContactEntity is the entity a contact.created envelope rides on.
const noticeCaseContactEntity = "contact"

// systemNoticeCaseActor names this consumer in the audit trail. A duty recorded
// against the workspace itself must say what recorded it: "who opened this
// case" is the first question an auditor asks of a compliance record.
const systemNoticeCaseActor = "system:notice-case-open"

// NoticeCaseOpen records the disclosure duty owed for a newly created contact.
type NoticeCaseOpen struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

// NewNoticeCaseOpen builds the consumer. The clock is injected because the
// deadline is the whole point of the row: a test that had to wait a month to
// prove one is a test nobody runs.
func NewNoticeCaseOpen(pool *pgxpool.Pool, now func() time.Time, log *slog.Logger) *NoticeCaseOpen {
	return &NoticeCaseOpen{pool: pool, now: now, log: log}
}

// HandleEvent records the duty for one created contact.
//
// Anything else answers nil so the consumer group keeps flowing rather than
// wedging on traffic this consumer ignores.
func (n *NoticeCaseOpen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID == ids.Nil || env.Entity.Type != noticeCaseContactEntity {
		return nil
	}
	// The generated constant, not a typed literal: a hand-written event name
	// that drifts from the contract makes this consumer silently never fire,
	// and nothing fails when a consumer does nothing.
	if env.Type != string(crmcontracts.ContactCreated) {
		return nil
	}
	db := InstallationDB(n.pool)
	ws, err := db.Workspace(ctx)
	if err != nil {
		return err
	}
	// A subscriber carries no workspace, no actor and no trace. Without the
	// actor the audited write below has nobody to name; without the correlation
	// id the audit row has no link back to the contact.created that caused it,
	// so a redelivery cannot be told from a second real event.
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   systemNoticeCaseActor,
	})
	return db.Tx(ctx, func(tx pgx.Tx) error {
		return n.openFor(ctx, tx, env.Entity.ID)
	})
}

// openFor records the duty for every acquisition this contact has that does not
// already carry one.
//
// EVERY acquisition, not the first. The table is keyed per acquisition because a
// contact obtained twice owes the duty twice, and a merge MOVES evidence onto
// the survivor — so by the time this runs a contact may hold several rows, and
// picking one would leave the rest owed and unrecorded forever. Nothing else
// ever looks at them again: contact.created fires once.
//
// That also repairs the merge race. A contact merged away between the event and
// this handler has had their evidence moved to the survivor, so a lookup by the
// retired id finds nothing and acknowledges — but the survivor's own pass, or
// any later contact.created for them, sweeps up the moved row because it is
// still an acquisition with no case.
//
// A contact with no acquisition row is not an error: the row is written by the
// creation doors, and a contact created by a path predating them has none.
// Recording a duty from no evidence would be inventing one.
func (n *NoticeCaseOpen) openFor(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.kind, coalesce(a.occurred_at, a.captured_at)
		  FROM contact_acquisition_evidence a
		 WHERE a.contact_id = $1
		   AND NOT EXISTS (
		         SELECT 1 FROM privacy_notice_case c
		          WHERE c.acquisition_id = a.id)
		 ORDER BY a.captured_at, a.id`, contactID)
	if err != nil {
		return fmt.Errorf("read the acquisitions this contact arrived by: %w", err)
	}
	type owed struct {
		id       ids.UUID
		kind     string
		occurred *time.Time
	}
	var pending []owed
	for rows.Next() {
		var o owed
		if err := rows.Scan(&o.id, &o.kind, &o.occurred); err != nil {
			rows.Close()
			return fmt.Errorf("read an acquisition: %w", err)
		}
		pending = append(pending, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the acquisitions this contact arrived by: %w", err)
	}
	if len(pending) == 0 {
		n.log.DebugContext(ctx, "no acquisition owes a notice case",
			slog.String("contact_id", contactID.String()))
		return nil
	}

	for _, o := range pending {
		duty, isOwed := consent.DutyFor(o.kind)
		if !isOwed {
			continue
		}
		// The clock runs from the ACQUISITION, not from now: an import landing
		// today carrying last year's business card is already late, and dating
		// the duty from the import would restart a clock that has been running
		// for a year.
		//
		// consent.AddMonths, not a span of hours and not raw AddDate: a fixed
		// 30 days is short in a 31-day month, and AddDate normalizes 31 January
		// into 3 March. AddMonths clamps to the last day of the target month,
		// which is what "one month" means when the day does not exist in it.
		from := n.now()
		if o.occurred != nil {
			from = *o.occurred
		}
		if err := consent.OpenNoticeCaseTx(ctx, tx, consent.NoticeCaseInput{
			ContactID:     ids.From[ids.ContactKind](contactID),
			AcquisitionID: o.id,
			Rule:          duty.Rule,
			DueAt:         consent.AddMonths(from, duty.Months),
			AllowedRoutes: duty.Routes,
			State:         duty.State,
		}); err != nil {
			return err
		}
	}
	return nil
}
