// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// What an EMPTY radius answer owes its reader, split from querygeo.go when that
// file reached its length ceiling.
//
// The boundary is a real one. querygeo.go answers "which records are within
// this circle"; this answers a different question that only arises when that
// one came back with nothing — whether the emptiness is a fact about the
// customers or a fact about the geocoder. A zero-row radius is honest either
// way, and only one of the two is safe to report as a reading of the workspace.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// geocodeReach is what the caller could have matched, had the radius been wide
// enough — and the third answer is the one a bool could not carry.
//
// "Nothing is placed" and "I could not find out" both come back from a
// two-valued probe as false, and only the first of them owes the note. A
// revoked grant answering the same as an empty geocoder publishes "records
// carry no normalized coordinates yet" about a deployment whose records are
// fine.
type geocodeReach int

const (
	// reachUnanswerable: the question could not be put. No note — the empty
	// answer's reason is elsewhere and naming the geocoder would invent one.
	reachUnanswerable geocodeReach = iota
	// reachNone: nothing the caller can read carries usable coordinates, so an
	// empty radius says nothing about their customers.
	reachNone
	// reachSome: at least one readable row is placed, so the empty radius is a
	// real reading.
	reachSome
)

// anythingLocated reports whether the caller could have matched ANYTHING, had
// the radius been wide enough: a row of the target type, inside their own row
// scope, whose coordinates mean something.
//
// WHY THE QUESTION IS ASKED OF THE ROWS AND NOT THE SCHEMA. bindGeo settles
// whether the deployment CAN rank by distance by looking at the columns, and
// that was read as the whole capability. It is half of it: a workspace whose
// companies all sit at geocode_status 'stale' runs the statement, matches
// nothing, and answers zero rows at coverage complete_exact with no note — a
// search that failed short in the shape of a complete, exact answer. The caller
// then reports that nobody is near the place asked about, which is a claim
// about the customers made from a fact about the geocoder.
//
// IT CARRIES THE ANSWER'S OWN SCOPE, and that is the whole reason it is built
// from the branch rather than from the type name. A first cut probed the table
// workspace-wide, reasoning that the capability is a property of the
// deployment. It is not: the answer being corrected is row-scoped, so a rep
// whose own accounts are all unplaced, in a workspace where a colleague's are
// geocoded, would have been told nothing was near them and given no note —
// which is this defect exactly, aimed at the caller likeliest to meet it.
//
// ON THE EMPTY PATH ONLY, which is why it is a second query rather than a
// widening of the first. An answer that came back with rows has proved the
// capability by using it, and pre-checking would put a count in front of every
// radius call to say something the rows themselves say.
func (e *QueryExecutor) anythingLocated(ctx context.Context, target string) (geocodeReach, error) {
	columns, locatable := geoCapableTargets[target]
	if !locatable {
		// bindGeo already refused this plan with its own note, so nothing here
		// is owed a second one.
		return reachUnanswerable, nil
	}
	branch, ok := branchFor(target)
	if !ok {
		return reachUnanswerable, fmt.Errorf("search: %q is not a searchable record type", target)
	}
	var args []any
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	scope, admitted, err := branchScope(ctx, branch, "t", arg)
	if err != nil {
		return reachUnanswerable, err
	}
	if !admitted {
		// Object RBAC stopped admitting the type mid-flight. The empty answer
		// already has its reason and it is not the geocoder's — a note saying
		// "records carry no normalized coordinates yet" would be a false
		// statement about the deployment, which is this note's own defect
		// pointed the other way.
		return reachUnanswerable, nil
	}
	// Every identifier is a compile-time literal off the branch and
	// geoCapableTargets; the status value and the scope's terms bind as
	// parameters.
	where := []string{"t.archived_at IS NULL", fmt.Sprintf("t.%s = $%d", columns.Status, arg(geoResolvedStatus))}
	if narrowing := branch.narrowing("t"); narrowing != "" {
		where = append(where, narrowing)
	}
	if scope != "" {
		where = append(where, scope)
	}
	var located bool
	err = e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT EXISTS (SELECT 1 FROM %s t WHERE %s)`, branch.table, strings.Join(where, " AND "),
		), args...).Scan(&located)
	})
	if err != nil {
		if budgetSpent(ctx, err) {
			// The probe is a scan bounded only by the row scope, over a table
			// with no placed rows — the very shape that runs long. Spending the
			// budget here must not turn an answer the caller already has into a
			// 500; it costs the note, which is the smaller loss.
			return reachUnanswerable, nil
		}
		return reachUnanswerable, fmt.Errorf("search: asking whether anything is geocoded: %w", err)
	}
	if located {
		return reachSome, nil
	}
	return reachNone, nil
}

// nothingToMatch answers the note an EMPTY radius answer owes, and whether it
// owes one at all.
//
// Asked only where the emptiness is — a plan with no radius, or one that came
// back with rows, has nothing to explain and pays nothing here. A radius that
// matched nothing over a scope that has placed nothing is not a statement about
// the customers, and returning it as coverage complete_exact with no note is
// how "nobody is near Cologne" gets said about a geocoder.
//
// The (value, ok, error) shape rather than this file's usual nil-pointer-means-
// absent: `nilnil` refuses a function returning both a nil pointer and a nil
// error, which is exactly the "nothing owed" case here.
func (e *QueryExecutor) nothingToMatch(
	ctx context.Context, target string, geo *geoBinding, rows []QueryRow,
) (QueryNote, bool, error) {
	if geo == nil || len(rows) > 0 {
		return QueryNote{}, false, nil
	}
	reach, err := e.anythingLocated(ctx, target)
	if err != nil {
		return QueryNote{}, false, err
	}
	if reach != reachNone {
		return QueryNote{}, false, nil
	}
	return QueryNote{
		Code:   CodeDistanceRankingUnavailable,
		Path:   geo.Field,
		Detail: unavailableDetail(CodeDistanceRankingUnavailable),
	}, true, nil
}
