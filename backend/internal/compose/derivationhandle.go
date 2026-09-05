// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The derivation handle's WIRE form: minting the URL that rides beside every
// aggregate, and reading it back.
//
// Split from derivation.go, which resolves a parsed handle against a report's
// vocabulary. The two change for unrelated reasons — a new query-string key is
// this file, a new validation rule is that one — and the round trip is the
// invariant this half owns: parseDerivationQuery is derivationURL's exact
// inverse, and a handle the product mints always resolves.

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// derivationURL mints the handle for one aggregate row (or, with a nil
// row, for the whole filtered result). parseDerivationQuery is its exact
// inverse; the round trip is unit-tested so a handle we mint always
// resolves.
func derivationURL(
	report string, filters map[string]any, groupBy []string,
	aggregates []reportAggregate, row map[string]any, asOf time.Time,
) string {
	values := url.Values{}
	if !asOf.IsZero() {
		values.Set(asOfKey, asOf.UTC().Format(time.RFC3339Nano))
	}
	for _, agg := range aggregates {
		values.Add("agg", agg.Fn+":"+agg.Field+":"+agg.As)
	}
	filterKeys := make([]string, 0, len(filters))
	for key := range filters {
		filterKeys = append(filterKeys, key)
	}
	sort.Strings(filterKeys)
	for _, key := range filterKeys {
		setPredicate(values, key, filters[key])
	}
	// The dimensions are NAMED whether or not a row pins their values, so that a
	// drill-through scopes the same references its headline did. A result handle
	// silent about its grouping could not tell a report grouped BY partner —
	// which excludes the partners its reader cannot open — from one grouped by
	// stage, which keeps them, and then opened rows the count never counted.
	for _, dim := range groupBy {
		values.Add("by", dim)
		if row != nil {
			setPredicate(values, dim, row[dim])
		}
	}
	return "/v1/reports/" + url.PathEscape(report) + "/derivation?" + values.Encode()
}

// setPredicate writes one bound field into the handle: an unset column is named
// under nullPredicateKey, anything else carries its rendered value. Keeping the
// two apart is what stops the empty string from reading as NULL.
//
//craft:ignore naked-any handle values arrive from JSON plan echoes and wire-shaped report rows — schemaless by design
func setPredicate(values url.Values, key string, v any) {
	if v == nil {
		values.Add(nullPredicateKey, key)
		return
	}
	values.Set(key, predicateString(v))
}

// predicateString renders a bound value for the handle's query string. Never
// called with nil — setPredicate spells that case separately.
//
//craft:ignore naked-any handle values arrive from JSON plan echoes and wire-shaped report rows — schemaless by design
func predicateString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// parseDerivationQuery reads a handle's query string back into the
// typed form: `by` and `agg` are the reserved plan keys, every other
// parameter is an equality predicate to be validated against the
// report's closed vocabulary.
func parseDerivationQuery(values url.Values) (derivationQuery, error) {
	q := derivationQuery{Predicates: map[string]string{}, Unset: map[string]bool{}}
	for key, vals := range values {
		switch key {
		case "by":
			q.GroupBy = append(q.GroupBy, vals...)
		case nullPredicateKey:
			for _, field := range vals {
				q.Unset[field] = true
			}
		case asOfKey:
			at, err := parseHandleAsOf(vals)
			if err != nil {
				return derivationQuery{}, err
			}
			q.AsOf = at
		case "agg":
			aggregates, err := parseHandleAggregates(vals)
			if err != nil {
				return derivationQuery{}, err
			}
			q.Aggregates = append(q.Aggregates, aggregates...)
		default:
			if len(vals) != 1 {
				// One cell binds one value per field; a repeated key is
				// not a plan this engine ever minted.
				return derivationQuery{}, &FieldNotAllowedError{Field: key}
			}
			q.Predicates[key] = vals[0]
		}
	}
	sort.Strings(q.GroupBy)
	for field := range q.Unset {
		// One field cannot be both unset and equal to something; a handle
		// saying both is not one this engine ever minted.
		if _, ok := q.Predicates[field]; ok {
			return derivationQuery{}, &FieldNotAllowedError{Field: field}
		}
	}
	return q, nil
}

// parseHandleAsOf reads the instant the headline was computed at. One value,
// RFC3339 with nanoseconds — the format derivationURL writes.
func parseHandleAsOf(vals []string) (time.Time, error) {
	if len(vals) != 1 {
		return time.Time{}, &FieldNotAllowedError{Field: asOfKey}
	}
	at, err := time.Parse(time.RFC3339Nano, vals[0])
	if err != nil {
		return time.Time{}, &FieldNotAllowedError{Field: asOfKey + "=" + vals[0]}
	}
	return at, nil
}

// parseHandleAggregates reads the `fn:field:as` triples. The field may be empty
// (count takes none); the function may not, or the handle names no aggregate.
func parseHandleAggregates(vals []string) ([]reportAggregate, error) {
	out := make([]reportAggregate, 0, len(vals))
	for _, v := range vals {
		parts := strings.SplitN(v, ":", 3)
		if len(parts) != 3 || parts[0] == "" {
			return nil, &FieldNotAllowedError{Field: "agg=" + v}
		}
		out = append(out, reportAggregate{Fn: parts[0], Field: parts[1], As: parts[2]})
	}
	return out, nil
}
