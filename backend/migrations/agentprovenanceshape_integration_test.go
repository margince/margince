// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// An agent-scheduled message says which agent scheduled it, and the DATABASE is
// what says so.
//
// The point of asserting it here rather than in the store is what the assertion
// survives. A store test proves today's writer fills the columns; it proves
// nothing about tomorrow's, and a writer that forgot them used to degrade in
// silence — the fire path read the absent actor as "cannot say which agent this
// was" and derived `agent:<human-uuid>`, an identity that never existed and
// that every agent acting for one person collapses into. Making omission a
// REFUSAL is what stops that being reachable at all.
//
// Both directions, because a constraint that refused everything would pass the
// refusal half on its own: the same row with an actor id is accepted, and a
// human row with no provenance is accepted too.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// scheduleAs inserts one scheduled_send of the given principal kind, with the
// agent columns exactly as given, and answers what the database said.
//
// The columns that are not the subject carry the cheapest values the table's
// OTHER constraints admit — an account-kind origin with an empty link array,
// which scheduled_send_origin_shape requires — so a refusal here can only be
// the provenance shape.
func scheduleAs(t *testing.T, owner *pgx.Conn, kind string, actorID *string) error {
	t.Helper()
	// A scheduler of this case's own. The address is drawn rather than spelled
	// because app_user.email is unique and these cases differ by a value that
	// does not make a distinct one — two of them name an actor.
	var scheduledBy string
	if err := owner.QueryRow(context.Background(),
		`INSERT INTO app_user (email, display_name, status)
		 VALUES (gen_random_uuid()::text || '@example.test', 'Provenance Scheduler', 'active')
		 RETURNING id`).Scan(&scheduledBy); err != nil {
		t.Fatalf("seeding the scheduling user: %v", err)
	}
	_, err := owner.Exec(context.Background(), `
		INSERT INTO scheduled_send
		    (id, scheduled_at, scheduled_tz, origin_kind, origin_links, payload,
		     scheduled_by, principal_kind, agent_actor_id)
		VALUES (gen_random_uuid(), now() + interval '1 hour', 'Europe/Berlin',
		        'account', '[]'::jsonb, '{}'::jsonb, $1, $2, $3)`,
		scheduledBy, kind, actorID)
	return err
}

func TestAnAgentScheduledSendMustNameTheAgent(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)

	// THE REFUSAL. This row is what a writer that forgot the provenance columns
	// produces, and before the narrowed constraint the table took it.
	err := scheduleAs(t, owner, "agent", nil)
	if err == nil {
		t.Fatal("an agent-kind scheduled send with no agent_actor_id was accepted — " +
			"the row is indistinguishable from one whose writer forgot, and the fire " +
			"path has no way to tell which agent asked for it")
	}
	if !strings.Contains(err.Error(), "scheduled_send_agent_provenance_shape") {
		t.Fatalf("the insert was refused by something other than the provenance shape: %v", err)
	}

	// A BLANK actor is the same hole with a different spelling: the writers
	// pass principal.ID through unchecked, and IS NOT NULL alone admits "".
	blank := "   "
	if err := scheduleAs(t, owner, "agent", &blank); err == nil {
		t.Error("an agent-kind scheduled send whose actor id is whitespace was accepted — " +
			"it names nobody, exactly as a NULL does")
	}

	// THE ALLOW ARM, same row, one column moved: a named agent is accepted.
	actor := "agent:01J0000000000000000000000A"
	if err := scheduleAs(t, owner, "agent", &actor); err != nil {
		t.Fatalf("an agent-kind scheduled send naming its agent was refused: %v", err)
	}

	// And a HUMAN row still carries no agent provenance at all, which is the
	// half the narrowed constraint must not have taken away.
	if err := scheduleAs(t, owner, "human", nil); err != nil {
		t.Fatalf("a human-kind scheduled send with no agent columns was refused: %v", err)
	}
}
