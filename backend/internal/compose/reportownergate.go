// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The owner gate: what stands between a report that measures the whole
// installation and one named colleague's figures.
//
// A spec declaring measureEveryReadableRow over an identity table (deal,
// project) is narrowed by nothing, because row scope renders TRUE there. That
// is right for "how many projects are in delivery". It is wrong for "what did
// this colleague close", which asks about somebody specific. Whether the
// caller may measure that seat is the question resolveOwnerScope already
// answers for an explicit owner scope, so this file asks it through the same
// resolver rather than spelling a second lens.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// narrowedToMeasurableOwners is what an answer's envelope says when a
// breakdown by owner was narrowed to the owners the caller may measure.
const narrowedToMeasurableOwners = "owners_you_may_measure"

// ownerGated answers whether a spec's owner names pass through this gate.
// A spec on the caller's own population is already narrowed to the owners
// they may measure, so naming anyone else there answers nothing.
func ownerGated(spec reportSpec) bool {
	return spec.population == measureEveryReadableRow
}

// namesOwner answers whether a vocabulary name reads the column the
// population narrows on. Recognised by expression rather than by name, so a
// dimension called something else over the same column is still an owner.
//
// Held by: TestEveryInstallWideOwnerNameGoesThroughTheOwnerGate, which fails
// when an install-wide spec reads an owner some other way.
func namesOwner(spec reportSpec, field string) bool {
	return spec.dimensions[field] == colOwnerID || spec.filters[field] == colOwnerID
}

// requireMeasurableOwners refuses a filter naming an owner the caller may not
// measure.
//
// A refusal rather than an empty answer: a rep asking for a colleague's revenue
// who is handed a zero reads it as "that colleague sold nothing".
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func requireMeasurableOwners(ctx context.Context, tx pgx.Tx, spec reportSpec, filters map[string]any) error {
	if !ownerGated(spec) {
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(filters)) {
		value := filters[key]
		if value == nil || !namesOwner(spec, key) {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return &FilterValueNotAllowedError{Filter: key, Kind: analyticsquery.JSONShapeOf(value)}
		}
		owner, err := ids.Parse(text)
		if err != nil {
			return fmt.Errorf("report filter %s names no seat: %w", key, apperrors.ErrInvalidArgument)
		}
		if err := requireMeasurableOwner(ctx, tx, owner); err != nil {
			return err
		}
	}
	return nil
}

// requireMeasurableOwner runs one named owner through the explicit owner
// scope's own lens.
//
// The lens answers 404 so a scope request cannot probe which teams exist. The
// refusal here is identical for every id the caller may not measure, real or
// not, so a 403 discloses nothing and says what actually happened.
func requireMeasurableOwner(ctx context.Context, tx pgx.Tx, owner ids.UUID) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("compose: no actor bound to context")
	}
	_, err := resolveAnalyticsScope(ctx, tx, p, RequestedScope{Kind: ScopeKindOwner, ID: &owner})
	if errors.Is(err, apperrors.ErrNotFound) {
		return fmt.Errorf("that owner's records are outside what you may measure: %w",
			apperrors.ErrPermissionDenied)
	}
	return err
}

// breaksDownByOwner answers whether a grouping splits the answer per owner.
//
// Not when it also groups by the row's own id: each row is then one record the
// caller may already open, owner and all, and narrowing it would drop from a
// listing records their ordinary list shows.
func breaksDownByOwner(spec reportSpec, groupBy []string) bool {
	byOwner, byRow := false, false
	for _, dim := range groupBy {
		switch spec.dimensions[dim] {
		case colOwnerID:
			byOwner = true
		case colRowID:
			byRow = true
		}
	}
	return byOwner && !byRow
}

// pinsOwner answers whether a set of filter names fixes the owner already, in
// which case requireMeasurableOwners has judged it and there is nothing left
// to narrow.
func pinsOwner(spec reportSpec, fields []string) bool {
	return slices.ContainsFunc(fields, func(field string) bool { return namesOwner(spec, field) })
}

// ownerBreakdownClause narrows a breakdown by owner to the owners the caller
// may measure, and names the reason the answer must carry. Both are "" when
// there is nothing to narrow.
//
// Narrowed rather than refused, so a manager's breakdown by rep still answers
// over their own teams. The clause is the caller's own default population, which
// resolves through the same lens as requireMeasurableOwner. An all-scope seat's
// default is the workspace, so it renders "" and nothing is announced.
func ownerBreakdownClause(
	ctx context.Context, tx pgx.Tx, spec reportSpec, breakdown bool, arg func(any) int,
) (clause, reason string, err error) {
	if !ownerGated(spec) || !breakdown {
		return "", "", nil
	}
	clause, err = reportPopulationClause(ctx, tx, callersOwnPopulation(), arg)
	if err != nil || clause == "" {
		return "", "", err
	}
	return clause, narrowedToMeasurableOwners, nil
}

// requireNamedRecords runs every gate a plan's filter VALUES answer to before
// anything binds: the read gate of a scoped record, then the owner gate. Both
// doors onto a report call it, so a drill-through handle cannot name what the
// plan that minted it could not.
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func requireNamedRecords(ctx context.Context, tx pgx.Tx, spec reportSpec, filters map[string]any) error {
	if err := requireFilterScopes(ctx, tx, spec, filters); err != nil {
		return err
	}
	return requireMeasurableOwners(ctx, tx, spec, filters)
}
