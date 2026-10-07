// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What the entity filter must SPELL, per entity type.
//
// The company timeline is the one asymmetric case: an account is reached
// through three links, so filtering on it emits the CompanyLinkedActivityExists
// walk instead of a flat activity_link join. Every other entity type is
// reached by its own link and only its own, so it keeps the join — and the
// row scope is composed either way, which is what keeps the wider account
// predicate from widening who may read.

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unscopedCtx binds the one principal for whom auth.ActivityContentClause
// contributes nothing, so the assertions below are about the entity filter
// alone. That principal is SYSTEM, not an admin: an activity's links reach
// contact and company rows, which carry capture privacy
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
func filterFor(ctx context.Context, t *testing.T, entityType string) (where string, args []any) {
	t.Helper()
	entity := ids.NewV7()
	terms, _, _, args, err := listActivitiesFilter(ctx, ListActivitiesInput{
		EntityType: &entityType, EntityID: &entity,
	})
	if err != nil {
		t.Fatalf("building the %s filter: %v", entityType, err)
	}
	return strings.Join(terms, " AND "), args
}

func TestCompanyFilterWalksTheAccountsFourArms(t *testing.T) {
	where, args := filterFor(unscopedCtx(), t, "company")
	if !strings.Contains(where, "EXISTS (") {
		t.Fatalf("company filter is not an EXISTS: %s", where)
	}
	for _, arm := range []string{
		"l.company_id = $1", "d.company_id = $1", "r.company_id = $1",
		"emp.company_id = $1",
	} {
		if !strings.Contains(where, arm) {
			t.Errorf("company filter misses the %q arm: %s", arm, where)
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
		t.Errorf("company filter bound %d args, want 1 (the account id): %v", len(args), args)
	}
}

// The reader's fourth arm is NOT the producer's. CompanyReachSet is what a signal
// is filed through, and somebody merely Cc'd on a message is weaker evidence
// than somebody the message was filed against — filing against every Cc'd
// contact's employer would put claims on accounts that were never in the
// conversation. The split is a ruling, so it is held rather than remembered.
func TestTheReachSetDoesNotFileThroughParticipants(t *testing.T) {
	set := CompanyReachSet()
	if strings.Contains(set, "activity_participant") {
		t.Errorf("CompanyReachSet reaches through participants, so a signal is now filed against "+
			"the employer of everybody copied on a message:\n%s", set)
	}
	// And the reader's does, or this test is only describing an absence that
	// was never a decision.
	if !strings.Contains(CompanyLinkedActivityExists(1), "activity_participant") {
		t.Error("the timeline predicate no longer reaches through participants either, " +
			"so the split above holds nothing apart")
	}
}

// The per-company shape is the bound walk with the enclosing company's column
// where the bind was: every arm, compared against the row the outer query is on.
func TestThePerCompanyWalkIsTheBoundWalkCorrelated(t *testing.T) {
	bound := strings.ReplaceAll(CompanyLinkedActivityExists(1), "$1", OuterCompanyAlias+".id")
	if got := CompanyLinkedActivityExistsPerCompany(); got != bound {
		t.Errorf("the per-company walk drifted from the bound one:\n got %s\nwant %s", got, bound)
	}
}

func TestEveryOtherEntityTypeKeepsItsFlatLinkJoin(t *testing.T) {
	for entityType, column := range map[string]string{
		"contact": "al.contact_id",
		"deal":    "al.deal_id",
		"lead":    "al.lead_id",
		"project": "al.project_id",
	} {
		where, args := filterFor(unscopedCtx(), t, entityType)
		// An EXISTS rather than a join, like the account arm: a join puts
		// activity_link's own created_at and id in the FROM list, where the
		// keyset tuple this page orders by cannot name its columns.
		if !strings.Contains(where, "EXISTS (SELECT 1 FROM activity_link al") {
			t.Errorf("%s filter is not an EXISTS over activity_link: %s", entityType, where)
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
	where, _ := filterFor(teamScopedCtx(), t, "company")
	// bool_or is the row-scope walk's own token: it is how the any-link
	// rule and the link-less-note rule are spelled in one pass, and the
	// account walk below never emits it.
	if !strings.Contains(where, "bool_or") {
		t.Errorf("the row-scope link-walk is missing from a bounded caller's account filter: %s", where)
	}
	// The employment arm is the walk's own token: the row-scope clause also
	// mentions l.company_id, so matching on that would pass vacuously.
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
// linked to a contact, a company and a deal is three activity_link rows, and a
// plain join would ask the reader to answer the same customer three times.
func TestTheWaitingQueryReturnsOneRowPerMessage(t *testing.T) {
	if !strings.Contains(waitingRepliesSQL, "GROUP BY a.id") {
		t.Fatal("the waiting query does not collapse a message's several links into one row")
	}
}

// Mail carries second precision, so two messages in one thread sharing a
// timestamp are ordinary. The newest-inbound walk breaks the tie by id, or two
// equal-second inbounds would both be the wait. An answer never does: an id
// says when a row was captured, not which message came first, so an answer
// must be strictly later and a tie stays owed.
func TestTheWaitingQueryBreaksTimestampTies(t *testing.T) {
	if strings.Count(waitingRepliesSQL, "newer.id) > (a.occurred_at, a.id)") == 0 {
		t.Fatal("the newest-inbound walk no longer breaks equal-second ties by id")
	}
	answers := answeredSQL("a", "$1")
	if strings.Contains(answers, ".id) > (a.occurred_at, a.id)") {
		t.Fatal("an answer is ordered by id, so capture order decides whether a same-second reply answered")
	}
	if regexp.MustCompile(`occurred_at\s*>=\s*a\.occurred_at`).MatchString(answers) {
		t.Fatal("an answer at the same second as the message counts as later")
	}
}

// Every later row the query reads is bounded by the read instant, so the answer
// is a snapshot. Mail carries the sender's own Date header: a message dated in
// the future must not suppress a thread that is genuinely waiting now.
func TestTheWaitingQueryIsBoundedByTheReadInstant(t *testing.T) {
	later := strings.Count(waitingRepliesSQL, ".id) > (a.occurred_at, a.id)") +
		strings.Count(waitingRepliesSQL, ".occurred_at > a.occurred_at")
	bounded := strings.Count(waitingRepliesSQL, "occurred_at <= $%[1]d")
	if later == 0 || bounded <= later {
		t.Fatalf("%d later-row comparisons but only %d bounds beyond the message's own: "+
			"a future-dated message can suppress a thread that is waiting now", later, bounded)
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
	matches := regexp.MustCompile(`(\w+)\.thread_key = a\.thread_key`).FindAllStringSubmatch(waitingRepliesSQL, -1)
	if len(matches) == 0 {
		t.Fatal("no thread match found — this gate is reading the wrong query")
	}
	for _, match := range matches {
		alias := match[1]
		if !strings.Contains(waitingRepliesSQL, alias+".kind = a.kind") {
			t.Errorf("%s matches the thread key without the medium", alias)
		}
		// The provider is compared two ways for one reason: the reply walks
		// NULL-match it, because two mail rows both having no provider is a
		// genuine match; the disposition join coalesces it to '' instead,
		// because a PRIMARY KEY cannot hold two NULLs as one row.
		if !strings.Contains(waitingRepliesSQL, alias+".channel_provider IS NOT DISTINCT FROM a.channel_provider") &&
			!strings.Contains(waitingRepliesSQL, alias+".channel_provider = coalesce(a.channel_provider, '')") {
			t.Errorf("%s matches the thread key without the provider: one matches across channels", alias)
		}
	}
}

// An unthreaded message is never matched loosely. It is also never excluded
// for being unthreaded.
//
// The danger is NULL-matching: `IS NOT DISTINCT FROM` joins every NULL to
// every other NULL, so one unthreaded outbound would silence every unthreaded
// question in the workspace. Plain equality never joins two NULLs, so each
// threadless row simply finds no reply and stays waiting — which is the honest
// answer about a message nobody answered.
//
// The query used to exclude such rows outright instead, on the reasoning that
// under-reporting by a row was cheaper. It was not cheaper, because this query
// is also the owed-verdict pass's backlog: an excluded row was never judged,
// so it never gained the verdict that would have let it back in.
func TestTheWaitingQueryNeverNullMatchesThreadKeys(t *testing.T) {
	// channel_provider is NULL-matched on purpose — a null provider means "not
	// a channel", and two mail rows both having none is a genuine match rather
	// than an accidental one. THREAD keys never are.
	if strings.Contains(waitingRepliesSQL, "thread_key IS NOT DISTINCT FROM") {
		t.Fatal("the waiting query matches NULL thread keys to each other")
	}
	// Every thread comparison is plain equality, which a NULL never satisfies.
	// Asked as a property of all of them rather than by naming one, so the
	// gate survives the query being reshaped: what matters is that no thread
	// match anywhere in it compares keys any other way.
	if strings.Count(waitingRepliesSQL, ".thread_key = a.thread_key") == 0 {
		t.Fatal("no thread match found — this gate is reading the wrong query")
	}
}

// The obligation admits threadless mail; request review does not.
//
// The waiting queue applies the machine-sender and colleague rules above its
// scan cap, so a threadless notification is dropped before anybody sees it.
// The deal card reads request review and applies neither, so there a
// threadless unjudged row would turn a hand-logged note into a standing
// obligation on the deal.
func TestOnlyTheObligationAdmitsThreadlessMail(t *testing.T) {
	if strings.Contains(owedSQL("$1", neverRelaxed, neverRelaxed), "a.thread_key IS NOT NULL") {
		t.Fatal("the obligation asks a message for a thread key, so a customer's threadless " +
			"mail cannot be judged and stays invisible")
	}
	if !strings.Contains(reviewableRequestSQL("$1"), "a.thread_key IS NOT NULL AND NOT") {
		t.Fatal("request review admits unjudged threadless mail without a sender rule in front of it")
	}
	// Every reader of the waiting query asks the same obligation, because the
	// query carries it rather than taking it from its caller.
	if !strings.Contains(waitingRepliesSQL, owedSQL("$%[1]d", "%[11]s", "%[17]s")) {
		t.Fatal("the waiting query no longer carries owedSQL, so the lane and the badge can disagree")
	}
}

// A threadless message reaches the rules that judge it.
//
// The regression this holds: an inbound message with no thread key was dropped
// before any other rule could read it, unless it already carried a capture
// label. The label comes from a classifier pass whose backlog is this very
// query, so the exclusion sustained itself — the row was never judged and so
// stayed excluded. On staging a client wrote "Dienstag 14 Uhr würde bei uns
// passen" and the deal card answered "Their move. Nobody here is owed an
// answer."
func TestAThreadlessMessageIsNotDroppedBeforeItIsJudged(t *testing.T) {
	if strings.Contains(waitingRepliesSQL, "AND (a.thread_key IS NOT NULL OR") {
		t.Fatal("a threadless message is dropped before the reply anti-joins can judge it, " +
			"so no pass can ever give it the evidence the same clause demands")
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
