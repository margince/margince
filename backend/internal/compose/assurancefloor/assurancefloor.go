// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package assurancefloor names the seats behind every figure the forecast's
// input check answers, so the small-group floor of "Show individual
// performance" can judge each one. compose hands in the floor and the
// population roster, which live in its root.
package assurancefloor

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Floor refuses an answer one of whose groups of seats falls below the floor
// while the switch is off.
type Floor func(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) error

// Members lists the seats a team or "My teams" total counts. counted is false
// for an owner or company scope, which has no roster.
type Members func(ctx context.Context, tx pgx.Tx, kind string, id *ids.UUID) (members []ids.UUID, counted bool, err error)

// finding is one finding as the floor reads it.
type finding struct {
	kind, subjectKind, currency string
	owner                       *ids.UUID
	claim, observed             []byte
}

// seats answers the seats behind one finding. A call's figures count the deals
// that moved across its scope. So its seats are the scope's members and those
// deals' owners, never the call's author alone.
func seats(ctx context.Context, tx pgx.Tx, members Members, f finding) ([]ids.UUID, error) {
	if f.subjectKind != assurance.SubjectKindForecastCall {
		if f.owner == nil {
			return nil, nil
		}
		return []ids.UUID{*f.owner}, nil
	}
	var claim struct {
		ScopeKind string    `json:"scope_kind"`
		ScopeID   *ids.UUID `json:"scope_id"`
	}
	var observed struct {
		DealIDs []ids.UUID `json:"deal_ids"`
	}
	if err := json.Unmarshal(f.claim, &claim); err != nil {
		return nil, fmt.Errorf("assurancefloor: reading a call finding's scope: %w", err)
	}
	if len(f.observed) > 0 {
		if err := json.Unmarshal(f.observed, &observed); err != nil {
			return nil, fmt.Errorf("assurancefloor: reading a call finding's deals: %w", err)
		}
	}
	out, _, err := members(ctx, tx, claim.ScopeKind, claim.ScopeID)
	if err != nil {
		return nil, err
	}
	if claim.ScopeKind == forecasting.ScopeOwner && claim.ScopeID != nil {
		out = append(out, *claim.ScopeID)
	}
	rows, err := tx.Query(ctx, `SELECT owner_id FROM deal WHERE id = ANY($1) AND owner_id IS NOT NULL`, observed.DealIDs)
	if err != nil {
		return nil, fmt.Errorf("assurancefloor: reading who owns a call's moved deals: %w", err)
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("assurancefloor: reading who owns a call's moved deals: %w", err)
	}
	return append(out, owners...), nil
}

// grouped lists the seats behind each figure a list of findings draws. Those
// are the whole list, each type, each type by severity or currency, and each
// call's own changes.
func grouped(ctx context.Context, tx pgx.Tx, members Members, found []finding, severities []string) ([][]ids.UUID, error) {
	byKey := map[string][]ids.UUID{"": {}}
	for i, f := range found {
		behind, err := seats(ctx, tx, members, f)
		if err != nil {
			return nil, err
		}
		keys := []string{"", "type:" + f.kind}
		if f.currency != "" {
			keys = append(keys, "currency:"+f.kind+":"+f.currency)
		}
		if severities != nil {
			keys = append(keys, "severity:"+f.kind+":"+severities[i])
		}
		if f.subjectKind == assurance.SubjectKindForecastCall {
			keys = append(keys, fmt.Sprintf("call:%d", i))
		}
		for _, key := range keys {
			byKey[key] = append(byKey[key], behind...)
		}
	}
	out := make([][]ids.UUID, 0, len(byKey))
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[key])
	}
	return out, nil
}

// ListGroups lists the seats behind each figure assurance.GroupFindings draws
// from a list of findings.
func ListGroups(ctx context.Context, tx pgx.Tx, members Members, found []assurance.Exception) ([][]ids.UUID, error) {
	read := make([]finding, 0, len(found))
	for _, e := range found {
		read = append(read, finding{kind: e.Type, subjectKind: e.SubjectKind, currency: e.Currency, owner: e.OwnerID, claim: e.Claim, observed: e.Observed})
	}
	return grouped(ctx, tx, members, read, nil)
}

// Preview holds a preview's figures to floor in the preview's own
// transaction: the deals it checked, each count, and every finding together.
func Preview(floor Floor, members Members) assurance.PreviewFloor {
	return func(ctx context.Context, tx pgx.Tx, contributions assurance.Contributions) error {
		read := make([]finding, 0, len(contributions.Raised))
		severities := make([]string, 0, len(contributions.Raised))
		for _, r := range contributions.Raised {
			claim, err := json.Marshal(r.Finding.Claim)
			if err != nil {
				return fmt.Errorf("assurancefloor: reading a finding's claim: %w", err)
			}
			observed, err := json.Marshal(r.Finding.Observed)
			if err != nil {
				return fmt.Errorf("assurancefloor: reading a finding's observation: %w", err)
			}
			read = append(read, finding{kind: r.Finding.Type, subjectKind: r.Finding.SubjectKind, owner: seatOf(r.Owner), claim: claim, observed: observed})
			severities = append(severities, r.Finding.Severity)
		}
		groups, err := grouped(ctx, tx, members, read, severities)
		if err != nil {
			return err
		}
		return floor(ctx, tx, append(groups, seatsOf(contributions.DealOwners)))
	}
}

// ReadRun answers the latest run and its sources in one snapshot, held to the
// floor there. The route and the forecast_input_checks tool both read it.
func ReadRun(ctx context.Context, pool *pgxpool.Pool, store *assurance.Store, floor Floor) (assurance.Run, []assurance.SourceCoverage, error) {
	var run assurance.Run
	var coverage []assurance.SourceCoverage
	err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		var err error
		if run, err = store.LatestRun(ctx); err != nil {
			return err
		}
		if coverage, err = store.CoverageFor(ctx, run.ID); err != nil {
			return err
		}
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			return requireRunFloor(ctx, tx, floor, run)
		})
	})
	return run, coverage, err
}

// requireRunFloor judges a run's counts by the seats behind each. A run
// recorded before its seats were kept cannot be judged, so it is withheld
// while the switch is off.
func requireRunFloor(ctx context.Context, tx pgx.Tx, floor Floor, run assurance.Run) error {
	if run.DealOwners == nil && run.EligibleDeals+run.EligibleSignals > 0 {
		allowed, err := identity.IndividualPerformanceFor(ctx, tx)
		if err != nil || allowed {
			return err
		}
		return auth.IndividualPerformanceOffError{}
	}
	return floor(ctx, tx, [][]ids.UUID{
		run.DealOwners, run.SignalOwners, slices.Concat(run.DealOwners, run.SignalOwners),
		run.Freshness.ChangedDealOwners, run.Freshness.ChangedSignalOwners,
	})
}

// seatOf is the seat an owner names, nil for an unowned subject.
func seatOf(owner string) *ids.UUID {
	seat, err := ids.Parse(owner)
	if err != nil {
		return nil
	}
	return &seat
}

// seatsOf is the seats a list of owners names.
func seatsOf(owners []string) []ids.UUID {
	out := make([]ids.UUID, 0, len(owners))
	for _, owner := range owners {
		if seat := seatOf(owner); seat != nil {
			out = append(out, *seat)
		}
	}
	return out
}
