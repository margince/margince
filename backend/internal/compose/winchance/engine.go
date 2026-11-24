// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package winchance reads a deal's one win chance for every surface that shows
// or weights it. These are the forecast, the deal reads, filters, reports, the
// Home queue and the tools. It also reads how accurate the forecast has been.
// The composition root injects what it cannot own.
package winchance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Deps is what the engine needs from the composition root.
type Deps struct {
	// RiskSubjects reads the assurance evidence of the given open deals as of an
	// instant, with the rules' configuration. It reads the stored probability and
	// never the win chance, so the chance cannot depend on itself.
	RiskSubjects func(ctx context.Context, tx pgx.Tx, at time.Time, only []ids.UUID) ([]assurance.Subject, assurance.Config, error)
	// PeriodAt resolves the forecast period of a kind that holds an instant.
	PeriodAt func(ctx context.Context, tx pgx.Tx, kind forecasting.PeriodKind, at time.Time) (forecasting.Period, error)
	// NoLoneColleague refuses, while individual performance is off, an answer
	// that one colleague's deals make up.
	NoLoneColleague func(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) error
	// ThinForAnyReader answers, per group of owners, whether the floor holds the
	// group's figure back from some reader while individual performance is off.
	ThinForAnyReader func(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) ([]bool, error)
	// MayAssertAbout answers whether the reader may see a population's frozen
	// forecasts, the rule every snapshot read follows.
	MayAssertAbout func(ctx context.Context, tx pgx.Tx, scope forecasting.Scope) (bool, error)
}

// Engine is the win chance reader.
type Engine struct{ Deps }

// New binds an engine to its injected reads.
func New(d Deps) *Engine { return &Engine{Deps: d} }
