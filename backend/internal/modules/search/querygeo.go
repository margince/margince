// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Answering "which customers are within 50km of Stuttgart".
//
// The predicate has been in the grammar the whole time and always came back
// `distance_ranking_unavailable`, because no row had a point to compare. Rows
// have points now, so the question is whether THIS deployment can answer it —
// and that is a capability, decided against the schema, not a constant.
//
// WHY THIS IS A BINDING RATHER THAN A DELETION. The validator used to append
// the unavailable note unconditionally, and Execute returned on any note before
// it ever consulted the schema reader. So "can we do distance" was answered
// twice, in two places, by neither of them looking. It is answered once here
// now: bindGeo asks the storage whether the target carries a coordinate pair,
// and the note it returns when the answer is no is the SAME honest note as
// before. A deployment that has not geocoded anything is not worse off; it is
// told the same thing, for a reason that is now true.
//
// NO EGRESS FROM HERE. A center given as a place NAME is resolved against the
// installation's place cache and nothing else. query_workspace is declared
// workspace-local and Scope.Egresses() is derived from that declaration
// precisely so a tool cannot quietly reach the internet; asking a geocoder here
// would make the declaration a lie. A name the cache does not hold answers a
// note that names the place it could not resolve, and the caller who wants it
// resolved uses the enrich-scoped door.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/margince/margince/backend/internal/platform/geocode"
)

// radius renders the SQL a bound radius predicate needs through geocode.Within.
func (c *planCompiler) radius(alias string, geo geoBinding) (distance string, where []string) {
	return geocode.Within(alias, geo.Columns, geo.Center, geo.RadiusKM, c.arg)
}

// locatableTarget reports whether this record type can be somewhere at all.
//
// Separate from bindGeo because the two questions settle at different times: a
// deal is never anywhere, which validation can say for the whole plan, while
// whether "Stuttgart" resolves depends on what this workspace has looked up
// and can only be answered per call.
func locatableTarget(target string) bool {
	_, ok := geocode.ColumnsFor(target)
	return ok
}

// geoBinding is a radius predicate this deployment CAN answer: where to measure
// from, how far, and which columns hold the point.
type geoBinding struct {
	Center   Point
	RadiusKM float64
	Columns  geocode.Columns
	// Field is the plan's own field name, so a note or an explain line can name
	// what the caller asked about rather than a column.
	Field string
}

// Point is the one point type, owned by platform/geocode.
type Point = geocode.Point

// PlaceResolver answers where a named place is, from what this INSTALLATION has
// already looked up.
//
// Installation-wide rather than per workspace, and deliberately: a place is a
// place whoever asks, so keying it by tenant would multiply every lookup by the
// number of them against a provider that holds this installation to four
// requests a minute. It is safe here because one active workspace per
// installation is an enforced invariant (identity.ErrMultipleWorkspaces) — a
// deployment that ever relaxed that would have to revisit this, since a cache
// hit and a miss are distinguishable from the outside.
//
// LOOKUP ONLY — there is deliberately no method that would resolve a name the
// cache does not hold. The seam has no door to the internet because the tool
// that reaches it has no cap to go there, and a resolver that could fetch would
// make that cap unenforceable by construction rather than by discipline.
type PlaceResolver interface {
	LookupPlace(ctx context.Context, query string) (Point, bool, error)
}

// bindGeo decides whether a radius predicate can be answered, and how.
//
// Three outcomes, and they are genuinely different things:
//   - bound: this deployment carries coordinates for the target and the center
//     resolved. The statement can measure.
//   - a NOTE: the deployment cannot answer this one — no coordinate columns, or
//     a place name nothing has looked up. Honest, and the answer says which.
//   - an error: something is broken.
func bindGeo(
	ctx context.Context, places PlaceResolver, target string, field string, operand radiusOperand,
) (*geoBinding, *Unavailable, error) {
	columns, locatable := geocode.ColumnsFor(target)
	if !locatable {
		// The record type has no place to be. Not a deployment gap — a
		// deal is not somewhere.
		return nil, &Unavailable{Path: field, Code: CodeDistanceRankingUnavailable}, nil
	}
	center, resolved, err := resolveCenter(ctx, places, operand)
	if err != nil {
		return nil, nil, err
	}
	if !resolved {
		return nil, &Unavailable{Path: field, Code: CodeDistanceRankingUnavailable}, nil
	}
	return &geoBinding{
		Center:   center,
		RadiusKM: *operand.RadiusKM,
		Columns:  columns,
		Field:    field,
	}, nil, nil
}

// resolveCenter answers where to measure from.
//
// Coordinates given directly are used as given — a caller who already holds a
// point has done the resolving, and second-guessing them would mean asking
// somebody about a place they can already name exactly.
func resolveCenter(ctx context.Context, places PlaceResolver, operand radiusOperand) (Point, bool, error) {
	if operand.Lat != nil && operand.Lon != nil {
		return Point{Lat: *operand.Lat, Lon: *operand.Lon}, true, nil
	}
	if places == nil {
		// No cache wired: this composition cannot turn a name into a point,
		// which is a deployment fact and not a fault.
		return Point{}, false, nil
	}
	return places.LookupPlace(ctx, operand.Center)
}

// A RADIUS BEATS SIMILARITY FOR ORDERING (Lars, 2026-08-21).
//
// "Companies like Kugellager, within 50km of Stuttgart" comes back NEAREST
// FIRST. Asking "within 50km" is asking about nearness, so distance orders the
// answer and similarity decides who qualifies for it — the reverse of the
// exact/similarity split everywhere else in this compiler, and deliberately so.
//
// That makes a THIRD sort lane. The two existing ones are: the exact lane,
// which orders in SQL by t.id, and the similarity lane, which emits no SQL
// ORDER BY at all because the retriever's ranking is applied in Go afterwards.
// A radius plan orders by the distance expression, in SQL, in both lanes —
// which is why orderByRank has to stop re-sorting when a distance is bound.
//
// STILL NOT DONE: ordering by a HOP's distance. The hop's LATERAL is
// `ORDER BY h.id LIMIT 1`, chosen so membership never depends on which related
// row came back; returning the nearest instead would change what a piece of
// evidence means. Filtering on a hop's radius is fine and works today — the
// alias is a parameter throughout — it is only the ordering that waits.
//
// The published refusal code is `distance_ranking_unavailable`, and it now
// means what it says: a deployment that cannot rank by distance says so, and
// one that can does not.

// bindGeoPredicate finds the plan's radius predicate, if it has one, and binds
// it against this deployment.
//
// ONE radius per plan, and a SECOND IS REFUSED rather than ignored.
//
// Two circles would mean an intersection, which the ordering cannot express —
// "nearest first" has no meaning when there are two centres. The first cut
// bound the first clause and let the compiler skip the rest, so a plan asking
// for companies near Stuttgart AND near Munich answered with everything near
// Stuttgart, in the shape of a correct answer. A wider answer that looks right
// is the worst failure available here, so the second clause refuses.
//
// A radius inside a TRAVERSAL is deliberately not bound here: filtering a hop
// by distance is a WHERE term the compiler already handles through the shared
// predicate path, and ordering by a hop's distance is the open question named
// at the top of this file.
func (e *QueryExecutor) bindGeoPredicate(
	ctx context.Context, plan ValidatedPlan,
) (*geoBinding, *Unavailable, error) {
	if at, found := secondRadius(plan.Plan.Where); found {
		return nil, &Unavailable{Path: at, Code: CodeDistanceRankingUnavailable}, nil
	}
	for i, clause := range plan.Plan.Where {
		if clause.Op != OpWithinRadius {
			continue
		}
		operand, ok := decodeRadiusOperand(clause.Value)
		if !ok {
			// Unreachable through Execute: the validator checked the operand's
			// shape against this same grammar. Reaching it means a plan that
			// never passed validation, which is a wiring fault rather than a
			// caller to explain anything to.
			return nil, nil, fmt.Errorf("search: where[%d] carries a radius operand that did not validate", i)
		}
		return bindGeo(ctx, e.places, plan.Target.Target, clause.Field, operand)
	}
	return nil, nil, nil
}

// decodeRadiusOperand reads the operand the validator already accepted.
func decodeRadiusOperand(raw json.RawMessage) (radiusOperand, bool) {
	var operand radiusOperand
	if err := json.Unmarshal(raw, &operand); err != nil {
		return radiusOperand{}, false
	}
	return operand, operand.namesACenter() && operand.RadiusKM != nil && *operand.RadiusKM > 0
}

// secondRadius finds a second radius predicate, and where it is.
//
// Reported rather than silently dropped: the clauses are ANDed, so a dropped
// one widens the answer without changing its shape — every company near
// Stuttgart returned for a question that asked for those ALSO near Munich, and
// nothing in the response saying so.
func secondRadius(clauses []Predicate) (string, bool) {
	seen := false
	for i, clause := range clauses {
		if clause.Op != OpWithinRadius {
			continue
		}
		if seen {
			return "where[" + strconv.Itoa(i) + "]", true
		}
		seen = true
	}
	return "", false
}
