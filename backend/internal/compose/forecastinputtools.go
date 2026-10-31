// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The forecast's input checks on the tool surface, each answered by the store
// the matching endpoint reads.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// assuranceToolReader answers forecast_input_checks, through the same store the
// endpoint reads.
func assuranceToolReader(pool *pgxpool.Pool) agents.AssuranceReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (json.RawMessage, error) {
		run, err := store.LatestRun(ctx)
		if err != nil {
			return nil, err
		}
		coverage, err := store.CoverageFor(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		out := agents.ForecastAssuranceResult{
			RunID:         run.ID.String(),
			AsOf:          run.AsOf.UTC().Format(time.RFC3339),
			Status:        run.Status,
			EligibleDeals: run.EligibleDeals,
			// Empty, never nil: a run that recorded no coverage is a real
			// answer, and null reads as "unknown" to a model.
			Sources:   []agents.ForecastAssuranceSourceResult{},
			Freshness: freshnessToolResult(assurance.FreshnessToWire(run.Freshness)),
		}
		if run.Readiness != nil {
			out.Readiness = *run.Readiness
		}
		if run.EligibleSignals > 0 {
			out.EligibleSignals = run.EligibleSignals
		}
		for _, c := range coverage {
			source := agents.ForecastAssuranceSourceResult{Source: c.Source, State: c.State}
			// Only a source actually read carries a date.
			if c.State == assurance.CoverageChecked && c.CheckedThrough != nil {
				source.CheckedThrough = c.CheckedThrough.UTC().Format(time.RFC3339)
			}
			out.Sources = append(out.Sources, source)
		}
		return json.Marshal(out)
	}
}

// freshnessToolResult is the endpoint's freshness in the tool's shape, so the
// two never decide "out of date" apart.
func freshnessToolResult(in crmcontracts.AssuranceFreshness) agents.ForecastAssuranceFreshnessResult {
	out := agents.ForecastAssuranceFreshnessResult{
		State: string(in.State), ChangedDeals: in.ChangedDeals, ChangedSignals: in.ChangedSignals,
		WorkspaceChanged: in.WorkspaceChanged,
	}
	if in.ChangedSince != nil {
		out.ChangedSince = in.ChangedSince.UTC().Format(time.RFC3339)
	}
	return out
}

// inputChecksToolReader answers list_input_checks, through the same scoped read
// the endpoint uses.
//
// The read goes through the deal's own visibility. So a finding about a deal
// the caller cannot open never reaches the tool's answer.
func inputChecksToolReader(pool *pgxpool.Pool) agents.InputChecksReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context, args agents.InputChecksArgs) (json.RawMessage, error) {
		population := assurance.Population{Kind: args.ScopeKind}
		if args.ScopeID != "" {
			id, err := ids.Parse(args.ScopeID)
			if err != nil {
				return nil, fmt.Errorf("scope_id is not a valid id: %w", apperrors.ErrInvalidArgument)
			}
			population.ID = &id
		}
		var found []assurance.Exception
		if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			found, err = AssuranceExceptionsIn(ctx, tx, population)
			return err
		}); err != nil {
			return nil, err
		}
		// Empty, never nil: null reads as "unknown" to a model. Here that is the
		// difference between a clean pipeline and an unread one.
		out := agents.InputChecksResult{Data: []agents.InputCheckResult{}}
		for _, e := range found {
			check := agents.InputCheckResult{
				ID: e.ID.String(), Type: e.Type, SubjectKind: e.SubjectKind,
				SubjectID: e.SubjectID.String(), Severity: e.Severity,
				AffectedMinor: e.AffectedMinor, Currency: e.Currency,
				Claim:         storedSlots(e.Claim),
				Observed:      storedSlots(e.Observed),
				FirstSeenAt:   e.FirstSeenAt.UTC().Format(time.RFC3339),
				LastSeenAt:    e.LastSeenAt.UTC().Format(time.RFC3339),
				InputsChanged: e.InputsChanged,
			}
			out.Data = append(out.Data, check)
		}
		return json.Marshal(out)
	}
}

// storedSlots passes a stored jsonb object through as it was written. An absent
// or malformed value becomes an empty object, because null reads as "unknown".
// One malformed row still names its deal and check, so it keeps the list up.
func storedSlots(raw []byte) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

// coverageToolReader answers data_coverage, through the same store the endpoint
// reads. The grant is checked in the store's own read, so a seller reaching
// this tool is refused by the same boundary the screen uses.
func coverageToolReader(pool *pgxpool.Pool) agents.SourceCoverageReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (json.RawMessage, error) {
		if err := auth.Require(ctx, "data_coverage", principal.ActionRead); err != nil {
			return nil, err
		}
		run, err := store.LatestRun(ctx)
		if err != nil {
			return nil, err
		}
		coverage, err := store.CoverageFor(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		out := agents.DataCoverageResult{
			RunID: run.ID.String(),
			AsOf:  run.AsOf.UTC().Format(time.RFC3339),
			// Empty, never nil: null reads as "unknown" to a model, which here
			// is a different claim from "nothing was tried".
			Sources: []agents.ForecastAssuranceSourceResult{},
		}
		for _, c := range coverage {
			source := agents.ForecastAssuranceSourceResult{Source: c.Source, State: c.State}
			if c.State == assurance.CoverageChecked && c.CheckedThrough != nil {
				source.CheckedThrough = c.CheckedThrough.UTC().Format(time.RFC3339)
			}
			out.Sources = append(out.Sources, source)
		}
		return json.Marshal(out)
	}
}
