// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The edge branch's SHAPE, which is where its two gates and its planner
// behaviour are decidable without a database: every branch carries both gates,
// the branches are disjoint, every placeholder is derived, and both arms of the
// union take the same keyset.
//
// The behaviour those properties buy — a withheld row, a full page, a withheld
// erasure — is proven against real SQL in the integration suite. What is here is
// what a rendered statement can be asked directly.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// edgeReaderContext is a row-scoped human who may read edges and their ends.
// Row-scoped on purpose: an unbounded caller renders an empty conjunction, so a
// test using one would assert nothing about the gate.
func edgeReaderContext(edgeGrant bool) context.Context {
	user := ids.NewV7()
	objects := map[string]principal.ObjectGrant{
		"person": {Read: true}, "organization": {Read: true},
		"deal": {Read: true}, "project": {Read: true},
	}
	if edgeGrant {
		objects["relationship"] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeOwn,
		},
	})
}

// renderEdgeCTE renders the CTE for one anchor kind and answers it with the
// arguments it registered.
func renderEdgeCTE(ctx context.Context, t *testing.T, entityType string) (string, []any, error) {
	t.Helper()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	anchorPos := arg(ids.NewV7())
	cte, err := edgeSubjectCTE(ctx, entityType, anchorPos, arg)
	return cte, args, err
}

func TestAnEdgelessCallerRegistersNoEdgeArguments(t *testing.T) {
	// The window drops the edge branch when the caller holds no edge grant, and
	// keeps the arguments it had already bound. If the denial arrived AFTER an
	// argument was registered, the record's own query would carry a placeholder
	// nothing supplies and every history read would 500.
	cte, args, err := renderEdgeCTE(edgeReaderContext(false), t, "person")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a caller with no edge grant: err = %v, want permission denied", err)
	}
	if cte != "" {
		t.Errorf("a refused caller was handed a CTE to run: %s", cte)
	}
	// One: the anchor id renderEdgeCTE bound itself.
	if len(args) != 1 {
		t.Errorf("the refused render registered %d argument(s), want only the caller's own", len(args))
	}
}

func TestAKindThatOccupiesNoEndpointColumnHasNoEdgeBranch(t *testing.T) {
	// A lead and an activity are never an end of a link. That is absence, not a
	// refusal and not an error, and it must cost no query.
	for _, entityType := range []string{"lead", entityTypeActivity} {
		cte, args, err := renderEdgeCTE(edgeReaderContext(true), t, entityType)
		if err != nil || cte != "" {
			t.Errorf("%s: cte = %q, err = %v, want no edge branch at all", entityType, cte, err)
		}
		if len(args) != 1 {
			t.Errorf("%s: registered %d argument(s) for a branch it does not render", entityType, len(args))
		}
	}
}

func TestEveryEdgeBranchGatesTheOtherEndsVisibilityAndErasure(t *testing.T) {
	// The organization anchor is the one with TWO anchor columns, so it renders
	// the most branches and is where a missing gate is most likely to hide.
	cte, args, err := renderEdgeCTE(edgeReaderContext(true), t, "organization")
	if err != nil {
		t.Fatalf("rendering the organization anchor's CTE: %v", err)
	}
	branches := strings.Split(cte, "UNION ALL")
	if want := len(edgeAnchorsFor("organization")); len(branches) != want {
		t.Fatalf("the organization anchor rendered %d branch(es), want one per column it can occupy (%d):\n%s",
			len(branches), want, cte)
	}
	for i, branch := range branches {
		// The visibility conjunction reaches every endpoint an edge can carry, so
		// each column must be named in each branch's scope clause.
		for _, endpoint := range edgeEndpoints {
			if !strings.Contains(branch, "r."+endpoint.column) {
				t.Errorf("branch %d does not constrain r.%s at all — an edge reaching a record "+
					"through that column would be answered ungated:\n%s", i, endpoint.column, branch)
			}
		}
		// One scrub filter per column that could hold the OTHER end. A branch
		// carrying fewer has an end whose erasure does not cut the row.
		if got, want := strings.Count(branch, "scrub.action = ANY("), len(edgeEndpoints)-1; got != want {
			t.Errorf("branch %d carries %d scrub-tombstone filter(s), want %d — one end's erasure would "+
				"leave its link's role and dates readable on the other record, after the erasure was "+
				"certified:\n%s", i, got, want, branch)
		}
	}
	// The scrub vocabulary is BOUND, never spelled into the statement — it is the
	// same list the anchor's own boundary uses, and two spellings of it would
	// diverge.
	if !argsContainScrubVocabulary(args) {
		t.Errorf("the scrub verbs were not bound as an argument: %v", args)
	}
}

func TestTheEdgeBranchesAreDisjointAndSargable(t *testing.T) {
	// Two properties of one line each, and they pull against each other: the
	// branches must not both match one edge, and the ONLY indexable predicate on
	// relationship must be the anchor's equality — a second one (a column required
	// to be null) is an access path the planner will take on a table where that
	// column usually is null.
	cte, _, err := renderEdgeCTE(edgeReaderContext(true), t, "organization")
	if err != nil {
		t.Fatalf("rendering the organization anchor's CTE: %v", err)
	}
	branches := strings.Split(cte, "UNION ALL")
	anchors := edgeAnchorsFor("organization")
	for i, branch := range branches {
		if got := strings.Count(branch, fmt.Sprintf("r.%s = $1", anchors[i].column)); got != 1 {
			t.Errorf("branch %d does not key on r.%s exactly once:\n%s", i, anchors[i].column, branch)
		}
		for _, other := range edgeEndpoints {
			// A bare "r.<col> IS NULL" conjunct — the scope clause's own
			// "r.<col> IS NULL OR EXISTS (…)" arm ends the line differently.
			if strings.Contains(branch, fmt.Sprintf("r.%s IS NULL\n", other.column)) {
				t.Errorf("branch %d requires r.%s to be null, which is a second indexable access "+
					"path on relationship:\n%s", i, other.column, branch)
			}
		}
		// Every earlier anchor column is excluded, and by IS DISTINCT FROM rather
		// than an equality the planner could ride.
		for _, earlier := range anchors[:i] {
			if !strings.Contains(branch, fmt.Sprintf("r.%s IS DISTINCT FROM $1", earlier.column)) {
				t.Errorf("branch %d does not exclude r.%s, so one edge can match two branches "+
					"and its rows appear twice:\n%s", i, earlier.column, branch)
			}
		}
	}
}

// placeholderPattern finds every bind position a rendered statement names.
var placeholderPattern = regexp.MustCompile(`\$(\d+)`)

func TestEveryPlaceholderInTheWindowIsDerivedFromTheArgumentList(t *testing.T) {
	// Nothing checks that a statement's placeholders and its arguments agree, so
	// this does: every $N the rendered window names must have a value behind it,
	// and no value may go unnamed.
	ctx := edgeReaderContext(true)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos, idPos := arg("organization"), arg(ids.NewV7())
	conds := []string{
		fmt.Sprintf("(a.occurred_at, a.id) >= ($%d, $%d)", arg("boundary-time"), arg(ids.NewV7())),
		fmt.Sprintf("(a.occurred_at, a.id) < ($%d, $%d)", arg("cursor-time"), arg(ids.NewV7())),
	}
	cte, err := edgeSubjectCTE(ctx, "organization", idPos, arg)
	if err != nil {
		t.Fatalf("rendering the CTE: %v", err)
	}
	sql := recordHistoryWindowSQL(typePos, idPos, arg(21), conds, cte)

	named := map[int]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(sql, -1) {
		position, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			t.Fatalf("unreadable placeholder %s", match[0])
		}
		if position < 1 || position > len(args) {
			t.Fatalf("the statement names $%d and only %d argument(s) are bound", position, len(args))
		}
		named[position] = true
	}
	for position := 1; position <= len(args); position++ {
		if !named[position] {
			t.Errorf("argument %d (%v) is bound and never named — Postgres refuses the statement",
				position, args[position-1])
		}
	}
}

func TestBothArmsOfTheWindowTakeTheSameKeyset(t *testing.T) {
	// A keyset applied to one arm and not the other pages the record's timeline
	// and its links at different speeds: rows repeat, and rows vanish.
	ctx := edgeReaderContext(true)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos, idPos := arg("person"), arg(ids.NewV7())
	keyset := fmt.Sprintf("(a.occurred_at, a.id) < ($%d, $%d)", arg("cursor-time"), arg(ids.NewV7()))
	cte, err := edgeSubjectCTE(ctx, "person", idPos, arg)
	if err != nil {
		t.Fatalf("rendering the CTE: %v", err)
	}
	sql := recordHistoryWindowSQL(typePos, idPos, arg(21), []string{keyset}, cte)

	if got := strings.Count(sql, keyset); got != 2 {
		t.Errorf("the keyset appears %d time(s) in the window, want once per union arm:\n%s", got, sql)
	}
	// The outer ORDER BY is what makes the two arms one timeline; each arm's own
	// LIMIT is what keeps the walk bounded.
	if !strings.Contains(sql, "ORDER BY occurred_at DESC, id DESC") {
		t.Errorf("the union is not ordered as one window:\n%s", sql)
	}
	if got := strings.Count(sql, "ORDER BY a.occurred_at DESC, a.id DESC"); got != 2 {
		t.Errorf("%d arm(s) walk the spine in order, want 2 — an unordered arm cannot stop at LIMIT", got)
	}
}

func TestAnEdgelessAnchorRendersTheRecordsOwnWindowAlone(t *testing.T) {
	// No edge branch means no CTE and no union: the read a lead or an activity
	// gets is the one it always had.
	sql := recordHistoryWindowSQL(1, 2, 3, nil, "")
	for _, unwanted := range []string{"WITH ", "UNION ALL", "relationship"} {
		if strings.Contains(sql, unwanted) {
			t.Errorf("an edgeless window still carries %q:\n%s", unwanted, sql)
		}
	}
	if !strings.Contains(sql, "a.entity_type = $1 AND a.entity_id = $2") {
		t.Errorf("the edgeless window lost its own keys:\n%s", sql)
	}
}

// argsContainScrubVocabulary reports whether the scrub verbs were bound rather
// than spelled into the statement.
func argsContainScrubVocabulary(args []any) bool {
	for _, value := range args {
		if verbs, isList := value.([]string); isList && len(verbs) == len(fieldHistoryScrubActions) {
			return true
		}
	}
	return false
}
