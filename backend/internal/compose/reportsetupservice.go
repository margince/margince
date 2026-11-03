// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/reportsetup"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The proposal's bounds. One model call may take this long. The model reads
// this many products at most: the live catalog's first page by name.
const (
	setupDeadline = 20 * time.Second
	setupProducts = 200
)

// initiativeObjectives are the metrics an initiative may be measured by, the
// same four the initiative drawer offers.
var initiativeObjectives = []crmcontracts.ReportingMetricID{
	"qualified_opportunities_created", "qualified_pipeline_created", reportingBookingsWon, reportingOpenPipeline,
}

// setupPeriods are the periods a proposal may name: every reporting period
// but a custom window, which the words cannot bound.
var setupPeriods = []string{"this_week", "last_week", "this_month", "last_month", "this_quarter"}

// productLister is the product catalog read under the reader's own grant.
type productLister interface {
	ListProducts(ctx context.Context, in deals.ListProductsInput) ([]crmcontracts.Product, storekit.Page, error)
}

// reportSetups turns a description into a proposal for one of the two
// drawers. It reads the catalog the reader may use, asks the report_setup
// lane once, and keeps only what the catalog holds. Nothing is written.
type reportSetups struct {
	reports  *reporting.Service
	products productLister
	pool     *pgxpool.Pool
	lane     completer
	now      func() time.Time
}

func newReportSetups(reports *reporting.Service, products productLister, pool *pgxpool.Pool) *reportSetups {
	return &reportSetups{reports: reports, products: products, pool: pool, now: time.Now}
}

func (s *reportSetups) withLane(lane completer) *reportSetups {
	s.lane = lane
	return s
}

// ProposeSetup answers one description for the reader. The drawer's own create
// grant is asked first, so a reader who could not save the result never
// spends a model call on it.
func (s *reportSetups) ProposeSetup(ctx context.Context, req crmcontracts.ReportSetupRequest) (crmcontracts.ReportSetupProposal, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ReportSetupProposal{}, err
	}
	if err := requireSetupCreate(ctx, req.Site); err != nil {
		return crmcontracts.ReportSetupProposal{}, err
	}
	description, err := httperr.RequireNonBlank("description", req.Description)
	if err != nil {
		return crmcontracts.ReportSetupProposal{}, err
	}
	if s.lane == nil {
		return crmcontracts.ReportSetupProposal{}, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeAINotConfigured,
			Detail: "a proposal from plain words needs an AI model — an administrator can bind one under Settings → AI; the setup can still be chosen by hand",
		}
	}
	in, err := s.gather(ctx, string(req.Site), description)
	if err != nil {
		return crmcontracts.ReportSetupProposal{}, err
	}
	laneCtx, cancel := context.WithTimeout(ctx, setupDeadline)
	defer cancel()
	res, err := ai.Ask(laneCtx, s.lane, reportsetup.Request(in), func(reply string) error {
		_, err := reportsetup.Parse(reply, in)
		return err
	})
	if err != nil {
		return crmcontracts.ReportSetupProposal{}, err
	}
	proposal, err := reportsetup.Parse(res.Text, in)
	if err != nil {
		return crmcontracts.ReportSetupProposal{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	return wireSetupProposal(req.Site, proposal, res.ServedModel)
}

// requireSetupCreate asks for the create grant on the record the drawer would make.
// Each object is a literal, so the grant census reads both doors.
func requireSetupCreate(ctx context.Context, site crmcontracts.ReportSetupRequestSite) error {
	if site == crmcontracts.ReportSetupRequestSiteInitiativeSetup {
		return auth.Require(ctx, "sales_initiative", principal.ActionCreate)
	}
	return auth.Require(ctx, "report_definition", principal.ActionCreate)
}

// gather reads the catalog the reader may use. That is the metrics the
// reporting catalog offers them and the charts those draw, the live
// products, the fixed industries and the periods.
func (s *reportSetups) gather(ctx context.Context, site, description string) (reportsetup.Input, error) {
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	in := reportsetup.Input{Site: site, Description: description, Today: s.now().UTC().Format(time.DateOnly), Lang: lang}
	catalog, err := s.reports.Catalog(ctx)
	if err != nil {
		return in, fmt.Errorf("read the reporting catalog: %w", err)
	}
	in.Catalog = setupCatalog(catalog, textlang.Lang(lang))
	products, err := s.liveProducts(ctx)
	if err != nil {
		return in, err
	}
	in.Catalog.Products = products
	return in, nil
}

func setupCatalog(catalog crmcontracts.ReportingCatalog, lang textlang.Lang) reportsetup.Catalog {
	out := reportsetup.Catalog{Periods: setupPeriods, Countries: contacts.AssignedCountryCodes()}
	var charts []string
	for _, metric := range catalog.Metrics {
		option := reportsetup.MetricOption{ID: string(metric.Id), Label: metric.Label}
		for _, block := range metric.Blocks {
			// The metric details table is drawn for every report and placed
			// by no one, so it is not a chart the words can ask for.
			if block == reportingMetricReading {
				continue
			}
			option.Charts = append(option.Charts, string(block))
			if !slices.Contains(charts, string(block)) {
				charts = append(charts, string(block))
			}
		}
		out.Metrics = append(out.Metrics, option)
		if slices.Contains(initiativeObjectives, metric.Id) {
			out.Objectives = append(out.Objectives, reportsetup.Option{ID: string(metric.Id), Label: metric.Label})
		}
	}
	for _, chart := range charts {
		out.Charts = append(out.Charts, reportsetup.Option{ID: chart, Label: chart})
	}
	for _, code := range contacts.IndustryCodes() {
		out.Industries = append(out.Industries, reportsetup.Option{ID: code, Label: contacts.IndustryLabel(code, lang)})
	}
	return out
}

// liveProducts are the active products by name. A reader without the
// product grant gets none, and the words name no product.
func (s *reportSetups) liveProducts(ctx context.Context) ([]reportsetup.Option, error) {
	active, limit, sort := true, setupProducts, "name"
	products, _, err := s.products.ListProducts(ctx, deals.ListProductsInput{Active: &active, Limit: &limit, Sort: &sort})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the product catalog: %w", err)
	}
	out := make([]reportsetup.Option, 0, len(products))
	for _, product := range products {
		out = append(out, reportsetup.Option{ID: product.Id.String(), Label: product.Name})
	}
	return out, nil
}

// wireSetupProposal is the proposal in the contract's shape.
func wireSetupProposal(site crmcontracts.ReportSetupRequestSite, proposal reportsetup.Proposal, served string) (crmcontracts.ReportSetupProposal, error) {
	out := crmcontracts.ReportSetupProposal{
		Site:           crmcontracts.ReportSetupProposalSite(site),
		GeneratedBy:    crmcontracts.WrittenByModel,
		Selection:      setupSelection(proposal),
		Sources:        make([]crmcontracts.ReportSetupSource, 0, len(proposal.Sources)),
		Clarifications: append([]string{}, proposal.Clarifications...),
		Unsupported:    make([]crmcontracts.ReportSetupUnsupported, 0, len(proposal.Unsupported)),
	}
	if proposal.Name != "" {
		out.Name = &proposal.Name
	}
	if served != "" {
		out.ModelUsed = &served
	}
	if proposal.Objective != nil {
		objective, err := setupObjective(*proposal.Objective)
		if err != nil {
			return out, err
		}
		out.Objective = &objective
	}
	for _, source := range proposal.Sources {
		item := crmcontracts.ReportSetupSource{Field: crmcontracts.ReportSetupSourceField(source.Field), Value: source.Value, Phrase: source.Phrase}
		if source.Label != "" {
			label := source.Label
			item.Label = &label
		}
		out.Sources = append(out.Sources, item)
	}
	for _, dropped := range proposal.Unsupported {
		out.Unsupported = append(out.Unsupported, crmcontracts.ReportSetupUnsupported{Phrase: dropped.Phrase, Code: crmcontracts.ReportSetupUnsupportedCode(dropped.Code), Reason: dropped.Reason})
	}
	return out, nil
}

func setupSelection(proposal reportsetup.Proposal) crmcontracts.ReportSetupSelection {
	var selection crmcontracts.ReportSetupSelection
	if proposal.Product != "" {
		if id, err := ids.Parse(proposal.Product); err == nil {
			product := crmcontracts.ReportingProductId(openapi_types.UUID(id))
			selection.ProductId = &product
		}
	}
	if proposal.Industry != "" {
		industry := crmcontracts.ReportingIndustry(proposal.Industry)
		selection.Industry = &industry
	}
	if proposal.Country != "" {
		country := crmcontracts.ReportingCountry(proposal.Country)
		selection.Country = &country
	}
	if proposal.Period != "" {
		period := crmcontracts.ReportSetupSelectionPeriod(proposal.Period)
		selection.Period = &period
	}
	if len(proposal.Metrics) > 0 {
		metrics := make([]crmcontracts.ReportingMetricID, 0, len(proposal.Metrics))
		for _, metric := range proposal.Metrics {
			metrics = append(metrics, crmcontracts.ReportingMetricID(metric))
		}
		selection.Metrics = &metrics
	}
	if len(proposal.Blocks) > 0 {
		blocks := make([]crmcontracts.ReportingBlockKind, 0, len(proposal.Blocks))
		for _, block := range proposal.Blocks {
			blocks = append(blocks, crmcontracts.ReportingBlockKind(block))
		}
		selection.Blocks = &blocks
	}
	return selection
}

func setupObjective(objective reportsetup.Objective) (crmcontracts.ReportSetupObjective, error) {
	out := crmcontracts.ReportSetupObjective{Metric: crmcontracts.ReportingMetricID(objective.Metric)}
	if objective.TargetValue > 0 {
		target := objective.TargetValue
		out.TargetValue = &target
	}
	if objective.Deadline != "" {
		day, err := time.Parse(time.DateOnly, objective.Deadline)
		if err != nil {
			return out, fmt.Errorf("%w: the proposed deadline is not a date: %w", ai.ErrOutputRejected, err)
		}
		out.Deadline = &openapi_types.Date{Time: day}
	}
	return out, nil
}
