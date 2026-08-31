// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// Admission: which connection a run spends against, and whether this trigger
// may spend at all. Separate from the queueing beside it because the two answer
// different questions — admission is about the installation's standing policy,
// queueing is about this one subject.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// ErrProviderAmbiguous reports that an automatic trigger found more than one
// connected provider and refused to guess which one to spend on. It is a
// configuration fault an operator must resolve, never a state to swallow.
var ErrProviderAmbiguous = errors.New("integrations: more than one provider is connected, so an automatic trigger cannot choose which to spend on")

// admittedConnection is the connection state a run freezes itself against.
type admittedConnection struct {
	id         string
	version    int64
	epoch      int64
	mode       string
	autoCreate bool
	autoImport bool
	categories []string
	refreshAge *int
	dailyLimit *int
}

// admit resolves the connection and refuses the triggers its policy does not
// admit. A connection that is not connected cannot spend anything, and a
// trigger the customer switched off must not spend on their behalf.
func (s *Store) admit(ctx context.Context, tx pgx.Tx, name string, trigger provider.Trigger) (admittedConnection, error) {
	var c admittedConnection
	err := tx.QueryRow(ctx, `
		SELECT id::text, version, execution_epoch, mode, automatic_individual_create,
		       automatic_import, categories, refresh_after_days, daily_run_limit
		  FROM provider_connection
		 WHERE provider = $1 AND status = 'connected'
		 FOR SHARE`, name).
		Scan(&c.id, &c.version, &c.epoch, &c.mode, &c.autoCreate,
			&c.autoImport, &c.categories, &c.refreshAge, &c.dailyLimit)
	if errors.Is(err, pgx.ErrNoRows) {
		return admittedConnection{}, provider.ErrNotConnected
	}
	if err != nil {
		return admittedConnection{}, fmt.Errorf("integrations: reading the connection: %w", err)
	}

	// One installation-wide answer governs every automatic trigger, in place of
	// the per-connection mode and the two switches beside it. Those asked which
	// WRITER a purchase followed — a human typing a contact, a connector
	// importing one — and the answer differed because a connector's thousands
	// of contacts each spent credits. An automatic run now buys only what costs
	// nothing, so that distinction stopped paying for itself, and what remains
	// is a legal question about the installation rather than about the writer.
	//
	// The columns are still read above and still answered on the wire; nothing
	// reads what they say.
	if trigger.Automatic() {
		on, err := automaticLookupEnabled(ctx, tx)
		if err != nil {
			return admittedConnection{}, err
		}
		if !on {
			return admittedConnection{}, errTriggerNotAdmitted
		}
	}
	return c, nil
}

// errTriggerNotAdmitted reports that the installation does not run automatic
// lookups. It is not an error the caller shows anybody: the person.created
// consumer swallows it, because a posture switched off is the configuration
// working, not a failure.
var errTriggerNotAdmitted = errors.New("integrations: automatic lookups are switched off for this installation")

// IsTriggerNotAdmitted reports whether a QueueRun refusal was the installation
// declining automatic lookups.
//
// It exists because the event consumer lives in compose and must tell that
// refusal apart from a failure, and the alternative is comparing error text:
// a predicate here means the sentinel can be reworded without silently
// turning a swallowed configuration state into a logged error.
func IsTriggerNotAdmitted(err error) bool {
	// ErrNothingFreeToBuy is the same kind of answer: the saved policy leaves
	// an automatic run nothing it may spend on, which is a configuration
	// working rather than a failure to log.
	return errors.Is(err, errTriggerNotAdmitted) || errors.Is(err, ErrNothingFreeToBuy)
}
