// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:cohort-promote consumer: a person's captured mail finds them however
// late they arrive.
//
// Capture links the message it ran the ensure for, which is the right scope at
// capture time and the wrong one afterwards. A workspace does not learn its
// contacts in the order the mail arrived: a Gmail backfill walks newest-first
// and creates the person somewhere in the middle of the run, a rep types a
// contact in months later, a verdict settles a question the sender kept writing
// during. Every message captured before that moment kept an address-only
// participant row and no link, and no reader of activity_link ever found it
// again — a company page reporting "no reply for 47 days" about someone who
// wrote last week.
//
// The trigger is the EVENT, not the writer. person.created and person.updated
// reach the outbox because the write shape puts them there, so manual entry,
// capture, channel ensure, lead promotion, CSV and vCard import, provider claims
// and merge all land here without any of them knowing this consumer exists — and
// a new writer added tomorrow is covered on the day it emits. Asking each writer
// to remember to repair the cohort would guarantee that one of them forgets,
// which is the defect this consumer exists to close rather than repeat.
//
// person.updated matters as much as person.created: an alias added to an
// existing contact is a new address, and the mail under it is exactly as
// stranded as a new person's would be. The pass re-derives the address set from
// person_email, so it needs nothing from the envelope but the id, and an update
// that touched no address is a cheap no-op.
//
// It lives in compose because the reaction crosses modules: the events are the
// people module's own, the seam that acts on them is nobody's private business.
//
// Idempotent by construction, so the at-least-once bus costs nothing: the link
// insert is ON CONFLICT DO NOTHING behind a "linked to nobody" guard, and the
// participant promotion only ever fills a row that names no person yet.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CohortPromoteGen repairs a person's captured cohort whenever that person
// appears or gains an address.
type CohortPromoteGen struct {
	pool  *pgxpool.Pool
	store *people.Store
	log   *slog.Logger
}

// NewCohortPromoteGen builds the repair consumer over the people store.
func NewCohortPromoteGen(pool *pgxpool.Pool, store *people.Store, log *slog.Logger) *CohortPromoteGen {
	return &CohortPromoteGen{pool: pool, store: store, log: log}
}

// HandleEvent routes one envelope to a repair pass. An event this consumer does
// not care about answers nil, so the group keeps flowing rather than wedging on
// somebody else's traffic.
func (g *CohortPromoteGen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.Type != personObject || env.Entity.ID == ids.Nil {
		return nil
	}
	// Every event that can leave a live person holding addresses whose mail is
	// not yet on their record. An archive needs no reaction: the repair only
	// ever ADDS a link, and an archived contact is not a record anybody is
	// asking to complete.
	if !personGainedAddresses(env.Type) {
		return nil
	}
	// The repair reads mail the workspace already holds and attaches it to a
	// record it already has; it creates nothing and admits nobody. The system
	// principal is what the sweeps that reach the same store method use, and
	// there is no human in an event consumer for object-RBAC to admit.
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   cohortPromoteActor,
	})
	person := ids.From[ids.PersonKind](env.Entity.ID)
	return InstallationDB(g.pool).Tx(ctx, func(tx pgx.Tx) error {
		done, err := g.store.PromotePersonCohortTx(ctx, tx, person)
		if err != nil {
			return err
		}
		if done.Linked > 0 || done.Promoted > 0 {
			g.log.InfoContext(ctx, "repaired a person's captured cohort",
				"person", person, "linked", done.Linked, "promoted", done.Promoted)
		}
		return nil
	})
}

// personGainedAddresses reports whether an event can leave a live person
// holding an address whose mail is not on their record yet — created, renamed
// or re-addressed, absorbed by a merge, or brought back.
func personGainedAddresses(eventType string) bool {
	switch eventType {
	case personCreatedEvent, personUpdatedEvent, personMergedEvent, personRestoredEvent:
		return true
	}
	return false
}

// The person events this repair reacts to. Spelled beside the predicate that
// reads them rather than at each case, so a fifth event is added in one place.
const (
	personUpdatedEvent  = "person.updated"
	personMergedEvent   = "person.merged"
	personRestoredEvent = "person.restored"
)

// cohortPromoteActor names this consumer in captured_by and in the audit trail,
// so a link that appeared without anybody clicking is attributable to the pass
// that wrote it rather than reading as the capture's own work.
const cohortPromoteActor = "cohort-promote"
