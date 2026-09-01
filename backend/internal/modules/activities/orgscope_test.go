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

func TestOrganizationFilterWalksTheAccountsFourArms(t *testing.T) {
	join, where, args := filterFor(unscopedCtx(), t, "organization")

	if join != "" {
		t.Errorf("organization filter joined %q; a join multiplies an activity "+
			"reachable through two links into two rows and breaks the keyset cursor", join)
	}
	if !strings.Contains(where, "EXISTS (") {
		t.Fatalf("organization filter is not an EXISTS: %s", where)
	}
	for _, arm := range []string{
		"l.organization_id = $1", "d.organization_id = $1", "r.organization_id = $1",
		"emp.organization_id = $1",
	} {
		if !strings.Contains(where, arm) {
			t.Errorf("organization filter misses the %q arm: %s", arm, where)
		}
	}
	if !strings.Contains(where, "r.kind = 'employment'") ||
		!strings.Contains(where, "r.ended_at IS NULL") {
		t.Errorf("the employment arm must be live employment only: %s", where)
	}
	// The participant arm reads employment the same way. A former colleague's
	// company is one they left, whether they were linked or invited.
	if !strings.Contains(where, "emp.kind = 'employment'") ||
		!strings.Contains(where, "emp.ended_at IS NULL") {
		t.Errorf("the participant arm must be live employment only: %s", where)
	}
	// Anchored on the activity in hand. Without it the arm asks "is anybody
	// anywhere a participant who works here", which is true of the whole
	// workspace at once.
	if !strings.Contains(where, "ap.activity_id = a.id") {
		t.Errorf("the participant arm is not anchored on the activity: %s", where)
	}
	// One bind position, read by all four arms: an account id registered
	// four times would silently mis-number every later placeholder.
	if len(args) != 1 {
		t.Errorf("organization filter bound %d args, want 1 (the account id): %v", len(args), args)
	}
}

// The reader's fourth arm is NOT the producer's. OrgReachSet is what a signal
// is filed through, and somebody merely Cc'd on a message is weaker evidence
// than somebody the message was filed against — filing against every Cc'd
// person's employer would put claims on accounts that were never in the
// conversation. The split is a ruling, so it is held rather than remembered.
func TestTheReachSetDoesNotFileThroughParticipants(t *testing.T) {
	set := OrgReachSet()
	if strings.Contains(set, "activity_participant") {
		t.Errorf("OrgReachSet reaches through participants, so a signal is now filed against "+
			"the employer of everybody copied on a message:\n%s", set)
	}
	// And the reader's does, or this test is only describing an absence that
	// was never a decision.
	if !strings.Contains(OrgLinkedActivityExists(1), "activity_participant") {
		t.Error("the timeline predicate no longer reaches through participants either, " +
			"so the split above holds nothing apart")
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

// A "my work" queue is MINE, and nobody else's.
//
// It used to admit unassigned work too, so a task a rep wrote themselves
// without an assignee still reached them. That kept one case and broke the
// queue for every other: an automation's follow-up carries no assignee either,
// and every colleague found it in their own day at once. The self-written task
// is answered by owning it as it is written (taskAssignee), not by widening
// what "mine" means.
func TestTheOwnQueueClauseIsExactlyTheReaders(t *testing.T) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	reader := ids.From[ids.UserKind](ids.MustParse("01a05500-0000-7000-8000-000000000001"))

	clause := ownQueueClause(&reader, arg)

	if strings.Contains(clause, "IS NULL") {
		t.Fatalf("the own-queue clause %q still admits work nobody owns", clause)
	}
	if !strings.Contains(clause, "a.assignee_id = $1") {
		t.Fatalf("the own-queue clause %q does not bind the reader", clause)
	}
}

// And ownerless work is still reachable, through a clause of its own. Making
// "mine" exact without this would not have moved that work — it would have
// hidden it.
func TestTheUnassignedQueueClauseAnswersOwnerlessWork(t *testing.T) {
	clause := unassignedQueueClause()

	if !strings.Contains(clause, "a.assignee_id IS NULL") {
		t.Fatalf("the unassigned clause %q does not select ownerless work", clause)
	}
	if !strings.Contains(clause, "NOT a.is_done") {
		t.Fatalf("the unassigned clause %q carries finished work", clause)
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
//
// Asked as a PROPERTY of every thread match rather than as a count of two.
// Counting pinned the query's shape at the moment the gate was written, so the
// disposition anti-join — a third, correct medium match — failed it for being
// new. What matters is that no comparison of thread keys stands alone.
func TestTheWaitingQueryMatchesWithinOneMedium(t *testing.T) {
	matches := strings.Count(waitingRepliesSQL, "thread_key = a.thread_key")
	if matches == 0 {
		t.Fatal("no thread match found — this gate is reading the wrong query")
	}
	if got := strings.Count(waitingRepliesSQL, "kind = a.kind"); got != matches {
		t.Fatalf("%d thread match(es) but %d carry the medium: one matches across media", matches, got)
	}
	// The provider is compared two ways for one reason: the reply anti-joins
	// NULL-match it, because two mail rows both having no provider is a genuine
	// match; the disposition join coalesces it to '' instead, because a
	// PRIMARY KEY cannot hold two NULLs as one row.
	providers := strings.Count(waitingRepliesSQL, "channel_provider IS NOT DISTINCT FROM a.channel_provider") +
		strings.Count(waitingRepliesSQL, "channel_provider = coalesce(a.channel_provider, '')")
	if providers != matches {
		t.Fatalf("%d thread match(es) but %d carry the provider: one matches across channels", matches, providers)
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

// The SELECT and the Scan must agree, column for column. They drifted when the
// sender was added in the middle of the list, and the read then failed at the
// database on every call — reported to the reader as "this source could not be
// read", which is indistinguishable from a permissions problem.
func TestTheWaitingQuerySelectsWhatItScans(t *testing.T) {
	from := strings.Index(waitingRepliesSQL, "FROM activity a")
	if from < 0 {
		t.Fatal("the waiting query no longer selects from activity")
	}
	head := waitingRepliesSQL[:from]
	subject := strings.Index(head, "a.subject")
	sender := strings.Index(head, "sender.address")
	occurred := strings.Index(head, "a.occurred_at")
	if subject >= sender || sender >= occurred {
		t.Fatal("the projection no longer reads id, subject, sender, occurred_at — the order Scan expects")
	}
}
