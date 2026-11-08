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

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
// The refusals of a run an agent asks for, which a user pressing the button
// does not meet. Nobody is at the screen to weigh the spend.
var (
	ErrAgentNeedsDailyLimit         = errors.New("integrations: an agent buys contact details only under a daily run limit; ask an administrator to set one on the connection")
	ErrConnectionNotHealthyForAgent = errors.New("integrations: the provider connection is out of credits, rate limited, failing or holds a key the provider refused, so an agent does not buy from it now")
)

// runIsAnAgents reports a run an agent asks for. It keeps the checks a user's
// press skips, because nobody weighed this purchase at a screen.
func runIsAnAgents(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	return ok && actor.Type == principal.PrincipalAgent
}

// notAdmitted says why no connection admitted the run. An agent is told the
// connection is unwell rather than absent, because the answer sends it elsewhere.
func (s *Store) notAdmitted(ctx context.Context, tx pgx.Tx, name string, byAgent bool) error {
	if !byAgent {
		return provider.ErrNotConnected
	}
	var degraded bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM provider_connection
		                WHERE provider = $1 AND status IN ('rate_limited', 'insufficient_credits', 'provider_error', 'invalid_credentials'))`,
		name).Scan(&degraded); err != nil {
		return fmt.Errorf("integrations: reading the connection: %w", err)
	}
	if degraded {
		return ErrConnectionNotHealthyForAgent
	}
	return provider.ErrNotConnected
}

func (s *Store) admit(ctx context.Context, tx pgx.Tx, name string, trigger provider.Trigger) (admittedConnection, error) {
	var c admittedConnection
	// A MANUAL run is admitted on a degraded connection; an automatic one is
	// not. The two are different questions.
	//
	// rate_limited, insufficient_credits and provider_error are recoverable
	// vendor conditions, and execution already says so in as many words: it
	// authorizes egress on them because "a later success is exactly what proves
	// recovery and restores connected". Admission refusing to CREATE that run
	// made the sentence unreachable — one bad minute from the vendor left the
	// connection refusing everything, with the only path out a human noticing
	// and re-entering a key that was never the problem.
	//
	// Automatic work stays refused: nobody is watching it, and a sweep retrying
	// a rate limit every minute is how a transient limit becomes a sustained
	// one. A contact pressing the button on a contact is the deliberate probe
	// this needs — they chose to spend it, and their run is what heals the
	// connection for everybody else.
	statuses := []string{"connected"}
	byAgent := runIsAnAgents(ctx)
	if !trigger.Automatic() && !byAgent {
		statuses = append(statuses, "rate_limited", "insufficient_credits", "provider_error")
	}
	err := tx.QueryRow(ctx, `
		SELECT id::text, version, execution_epoch, mode, automatic_individual_create,
		       automatic_import, categories, refresh_after_days, daily_run_limit
		  FROM provider_connection
		 WHERE provider = $1 AND status = ANY($2)
		 FOR SHARE`, name, statuses).
		Scan(&c.id, &c.version, &c.epoch, &c.mode, &c.autoCreate,
			&c.autoImport, &c.categories, &c.refreshAge, &c.dailyLimit)
	if errors.Is(err, pgx.ErrNoRows) {
		return admittedConnection{}, s.notAdmitted(ctx, tx, name, byAgent)
	}
	if err != nil {
		return admittedConnection{}, fmt.Errorf("integrations: reading the connection: %w", err)
	}
	if byAgent && c.dailyLimit == nil {
		return admittedConnection{}, ErrAgentNeedsDailyLimit
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
// lookups. It is not an error the caller shows anybody: the contact.created
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
