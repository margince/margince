// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What the entity filter must SPELL, per entity type.
//
// The company timeline is the one asymmetric case: an account is reached
// through three links, so filtering on it emits the OrgLinkedActivityExists
// walk instead of a flat activity_link join. Every other entity type is
// reached by its own link and only its own, so it keeps the join — and the
// row scope is composed either way, which is what keeps the wider account
// predicate from widening who may read.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unscopedCtx binds the one principal for whom auth.ActivityContentClause
// contributes nothing, so the assertions below are about the entity filter
// alone. That principal is SYSTEM, not an admin: an activity's links reach
// person and organization rows, which carry capture privacy
// (visibility='owner'), and capture privacy is a property of the row that
// row_scope=all does not clear — so a human admin does get a clause.
func unscopedCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// teamScopedCtx binds a rep bounded to one team, so the row scope clause is
// non-empty and its presence can be asserted.
func teamScopedCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		TeamIDs: []ids.UUID{ids.NewV7()},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	})
}

// filterFor builds the timeline filter for one entity type, failing the test
// on any error.
func filterFor(ctx context.Context, t *testing.T, entityType string) (join string, where string, args []any) {
	t.Helper()
	entity := ids.NewV7()
	joined, terms, _, args, err := listActivitiesFilter(ctx, ListActivitiesInput{
		EntityType: &entityType, EntityID: &entity,
	})
	if err != nil {
		t.Fatalf("building the %s filter: %v", entityType, err)
	}
	return joined, strings.Join(terms, " AND "), args
}

func TestOrganizationFilterWalksTheAccountsThreeLinks(t *testing.T) {
	join, where, args := filterFor(unscopedCtx(), t, "organization")

	if join != "" {
		t.Errorf("organization filter joined %q; a join multiplies an activity "+
			"reachable through two links into two rows and breaks the keyset cursor", join)
	}
	if !strings.Contains(where, "EXISTS (") {
		t.Fatalf("organization filter is not an EXISTS: %s", where)
	}
	for _, arm := range []string{"l.organization_id = $1", "d.organization_id = $1", "r.organization_id = $1"} {
		if !strings.Contains(where, arm) {
			t.Errorf("organization filter misses the %q arm: %s", arm, where)
		}
	}
	if !strings.Contains(where, "r.kind = 'employment'") ||
		!strings.Contains(where, "r.ended_at IS NULL") {
		t.Errorf("the employment arm must be live employment only: %s", where)
	}
	// One bind position, read by all three arms: an account id registered
	// three times would silently mis-number every later placeholder.
	if len(args) != 1 {
		t.Errorf("organization filter bound %d args, want 1 (the account id): %v", len(args), args)
	}
}

func TestEveryOtherEntityTypeKeepsItsFlatLinkJoin(t *testing.T) {
	for entityType, column := range map[string]string{
		"person":  "al.person_id",
		"deal":    "al.deal_id",
		"lead":    "al.lead_id",
		"project": "al.project_id",
	} {
		join, where, args := filterFor(unscopedCtx(), t, entityType)
		if join != " JOIN activity_link al ON al.activity_id = a.id" {
			t.Errorf("%s filter join = %q, want the flat activity_link join", entityType, join)
		}
		if !strings.Contains(where, "al.entity_type = $1") {
			t.Errorf("%s filter does not pin the link's entity_type: %s", entityType, where)
		}
		if !strings.Contains(where, column+" = $2") {
			t.Errorf("%s filter does not match %s: %s", entityType, column, where)
		}
		if strings.Contains(where, "r.kind = 'employment'") {
			t.Errorf("%s filter walks the account's employment arm; only an account does", entityType)
		}
		if len(args) != 2 {
			t.Errorf("%s filter bound %d args, want 2: %v", entityType, len(args), args)
		}
	}
}

// The account walk widens WHICH activities belong to the account. It must not
// widen WHO may read one, so the row-scope clause is still composed next to it.
func TestTheAccountWalkStillCarriesTheRowScope(t *testing.T) {
	_, where, _ := filterFor(teamScopedCtx(), t, "organization")
	// bool_or is the row-scope walk's own token: it is how the any-link
	// rule and the link-less-note rule are spelled in one pass, and the
	// account walk below never emits it.
	if !strings.Contains(where, "bool_or") {
		t.Errorf("the row-scope link-walk is missing from a bounded caller's account filter: %s", where)
	}
	// The employment arm is the walk's own token: the row-scope clause also
	// mentions l.organization_id, so matching on that would pass vacuously.
	if !strings.Contains(where, "r.kind = 'employment'") {
		t.Errorf("the account walk is missing for a bounded caller: %s", where)
	}
}

func TestAnUnknownEntityTypeIsRefusedRatherThanBuiltIntoSQL(t *testing.T) {
	entityType, entity := "invoice", ids.NewV7()
	_, _, _, _, err := listActivitiesFilter(unscopedCtx(), ListActivitiesInput{
		EntityType: &entityType, EntityID: &entity,
	})
	var badType *InvalidLinkTypeError
	if !errors.As(err, &badType) {
		t.Fatalf("filtering on an unknown entity type → %v, want InvalidLinkTypeError", err)
	}
	if badType.EntityType != entityType {
		t.Errorf("refusal names %q, want %q", badType.EntityType, entityType)
	}
}

// A "my work" queue is mine OR nobody's. A rep who writes themselves a task
// without filling in an assignee still owns it, and dropping it would hide the
// reader's own to-do from them.
func TestTheOwnQueueClauseAdmitsUnassignedWork(t *testing.T) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	reader := ids.From[ids.UserKind](ids.MustParse("01a05500-0000-7000-8000-000000000001"))

	clause := ownQueueClause(&reader, arg)

	if !strings.Contains(clause, "a.assignee_id IS NULL") {
		t.Fatalf("the own-queue clause %q drops unassigned work", clause)
	}
	if !strings.Contains(clause, "a.assignee_id = $1") {
		t.Fatalf("the own-queue clause %q does not bind the reader", clause)
	}
}

// The exact-assignment clause must stay exact: the task screen filters by it,
// and widening it there would put unassigned work on somebody's name.
func TestTheAssigneeClauseStaysExact(t *testing.T) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	assignee := ids.From[ids.UserKind](ids.MustParse("01a05500-0000-7000-8000-000000000001"))

	if clause := openTaskAssigneeClause(&assignee, arg); strings.Contains(clause, "IS NULL") {
		t.Fatalf("the exact-assignee clause %q now admits unassigned work", clause)
	}
}

// The waiting read is CONTENT, not a safe marker: everything it answers is
// derived from thread membership, so it composes the content clause. Reading
// through the discover gate would publish a wait, its timing and its linked
// record for a message the reader may not read — and let them watch the row
// vanish to learn a reply had arrived.
func TestTheWaitingQueryUsesTheContentGate(t *testing.T) {
	if !strings.Contains(waitingRepliesSQL, "%[2]s") {
		t.Fatal("the waiting query no longer binds an activity scope clause at all")
	}
	source, err := os.ReadFile("waiting.go")
	if err != nil {
		t.Fatalf("reading the waiting source: %v", err)
	}
	if !strings.Contains(string(source), "auth.ActivityContentClause") {
		t.Fatal("the waiting read admits rows through the discover gate, which is for safe markers only")
	}
	if !strings.Contains(string(source), "auth.LinkTargetVisibleClause") {
		t.Fatal("the waiting read returns links without checking the reader may see what they point at")
	}
}

// One message is ONE row however many records it is filed under. An activity
// linked to a person, a company and a deal is three activity_link rows, and a
// plain join would ask the reader to answer the same customer three times.
func TestTheWaitingQueryReturnsOneRowPerMessage(t *testing.T) {
	if !strings.Contains(waitingRepliesSQL, "GROUP BY a.id") {
		t.Fatal("the waiting query does not collapse a message's several links into one row")
	}
}

// Ties are broken by id. Mail carries second precision, so two messages in one
// thread sharing a timestamp are ordinary — and without the tie-break both
// halves of "newest inbound, no later outbound" are wrong at once.
func TestTheWaitingQueryBreaksTimestampTies(t *testing.T) {
	if strings.Count(waitingRepliesSQL, ".id) > (a.occurred_at, a.id)") != 2 {
		t.Fatal("the waiting query compares timestamps alone, so equal-second messages answer wrongly")
	}
}

// The anti-joins are bounded by the read instant, so the answer is a snapshot.
// Mail carries the sender's own Date header: a message dated in the future must
// not suppress a thread that is genuinely waiting now.
func TestTheWaitingQueryIsBoundedByTheReadInstant(t *testing.T) {
	if strings.Count(waitingRepliesSQL, "occurred_at <= $%[1]d") != 3 {
		t.Fatal("a future-dated message can suppress a thread that is waiting now")
	}
}

// A thread is matched within one medium. Mail thread keys come from headers the
// sender controls and share a namespace with channel keys, so comparing keys
// alone lets a crafted References value silence an unrelated conversation.
func TestTheWaitingQueryMatchesWithinOneMedium(t *testing.T) {
	if strings.Count(waitingRepliesSQL, "kind = a.kind") != 2 {
		t.Fatal("the waiting query matches threads across media")
	}
	if strings.Count(waitingRepliesSQL, "channel_provider IS NOT DISTINCT FROM a.channel_provider") != 2 {
		t.Fatal("the waiting query matches threads across channel providers")
	}
}

// An unthreaded message is excluded, not matched loosely.
//
// `IS NOT DISTINCT FROM` joins every NULL to every other NULL, so one
// unthreaded outbound would silence every unthreaded question in the
// workspace. Plain equality never joins two NULLs, so the rows would all
// survive and each would be its own thread. Neither is right, so they are
// excluded — which under-reports by a row rather than by a customer.
func TestTheWaitingQueryExcludesUnthreadedMessages(t *testing.T) {
	if !strings.Contains(waitingRepliesSQL, "a.thread_key IS NOT NULL") {
		t.Fatal("the waiting query admits unthreaded messages, which cross-suppress each other")
	}
	// Narrowly: THREAD keys must not be NULL-matched. channel_provider is
	// matched that way on purpose — a null provider means "not a channel", and
	// two mail rows both having none is a genuine match rather than an
	// accidental one.
	if strings.Contains(waitingRepliesSQL, "thread_key IS NOT DISTINCT FROM") {
		t.Fatal("the waiting query matches NULL thread keys to each other")
	}
}
