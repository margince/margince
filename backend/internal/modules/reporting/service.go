// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Fact retains contribution provenance so frozen values can be safely projected later.
type Fact struct {
	StagePosition int                            `json:"stage_position,omitempty"`
	Scope         *crmcontracts.ReportingScope   `json:"scope,omitempty"`
	StageID       string                         `json:"stage_id,omitempty"`
	StageLabel    string                         `json:"stage_label,omitempty"`
	OwnerLabel    string                         `json:"owner_label,omitempty"`
	Outcome       string                         `json:"outcome,omitempty"`
	Money         *int64                         `json:"money,omitempty"`
	Provenance    string                         `json:"provenance,omitempty"`
	Metric        crmcontracts.ReportingMetricID `json:"metric"`
	ContextID     string                         `json:"context_id"`
	GroupKey      string                         `json:"group_key"`
	// GroupLabel names GroupKey's group for a reader, in the base language.
	GroupLabel string                            `json:"group_label,omitempty"`
	Row        crmcontracts.ReportingEvidenceRow `json:"row"`
	SourceID   ids.UUID                          `json:"source_id"`
	SourceType string                            `json:"source_type"`
	OwnerID    ids.UUID                          `json:"owner_id"`
	// OwnerWithheld marks a fact whose owner the reader may not measure one by
	// one. It still counts in totals; per-owner outputs skip it. Absent on
	// stored contributions, which decode as false.
	OwnerWithheld bool `json:"owner_withheld,omitempty"`
	// PlacedAt is where a day row falls in a period: its date's start in the
	// installation zone. The row keeps the stored anchor a reader draws a date from.
	PlacedAt *time.Time `json:"placed_at,omitempty"`
}

// At is the moment that places the fact in a period or bucket.
func (f Fact) At() *time.Time {
	if f.PlacedAt != nil {
		return f.PlacedAt
	}
	return f.Row.OccurredAt
}

// Evaluation keeps calculated values and supporting facts in one result.
type Evaluation struct {
	Result crmcontracts.ReportingEvaluation
	Facts  []Fact
}

// Evaluator shares calculations across transport and publication boundaries.
type Evaluator interface {
	ProjectFrozen(crmcontracts.ReportingEvaluation, []Fact) (crmcontracts.ReportingEvaluation, error)
	Catalog(context.Context, textlang.Lang) (crmcontracts.ReportingCatalog, error)
	Evaluate(context.Context, pgx.Tx, crmcontracts.ReportingSelection, crmcontracts.ReportingFramework, time.Time) (Evaluation, error)
}

// Authority injects live identity and source visibility without sibling imports.
type Authority interface {
	MembersFor(context.Context, pgx.Tx, crmcontracts.ReportingScope) ([]ids.UUID, error)
	PublicationHuman(context.Context, pgx.Tx, ids.UUID) (context.Context, error)
	Pipeline(context.Context, pgx.Tx, *ids.UUID) error
	Members(context.Context, pgx.Tx) ([]ids.UUID, error)
	ValidateFramework(context.Context, pgx.Tx, crmcontracts.ReportingFrameworkInput) error
	Scope(context.Context, pgx.Tx, crmcontracts.ReportingScope, bool) (crmcontracts.ReportingScope, error)
	// AssertedScope resolves the scope a report revision records. A revision is
	// published about its population, so it needs the strict tier rule that a
	// read of the totals does not.
	AssertedScope(context.Context, pgx.Tx, crmcontracts.ReportingScope) (crmcontracts.ReportingScope, error)
	Visible(context.Context, pgx.Tx, []Fact) ([]Fact, bool, error)
}

// Calendar defines the installation’s civil-time and money context.
type Calendar struct {
	Timezone         string
	Currency         string
	FiscalStartMonth int
}

// CalendarSource reads calendar settings within the evaluation transaction.
type CalendarSource func(context.Context, pgx.Tx) (Calendar, error)

// Service coordinates saved reporting over injected calculation and authority seams.
type Service struct {
	language  func(context.Context, pgx.Tx) textlang.Lang
	reader    func(context.Context, pgx.Tx) textlang.Lang
	store     *store
	evaluator Evaluator
	authority Authority
	calendar  CalendarSource
	questions Questions
	now       func() time.Time
}

// NewService binds persistence, authorization and the clock once for every transport.
func NewService(db *database.DB, evaluator Evaluator, authority Authority, calendar CalendarSource, now func() time.Time) *Service {
	return &Service{store: &store{db: db}, evaluator: evaluator, authority: authority, calendar: calendar, now: now}
}

type store struct{ db *database.DB }
