// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The two report commands: running one, and composing a document of several.
//
// Together because they are one concept — an aggregate over rows the caller's
// own scope already bounds — and both resolvers answer the same way: NO record.
// There is no row an approval could bind to or be probed against, so what each
// supplies instead is the one fact that says which aggregate is being released.

import (
	"context"
	"fmt"
)

// RunReportCommand is one report run, whichever door asked for it. The report
// KEY is the whole of it: the plan arguments narrow what is counted, and the
// engine — not this seam — owns which of them a report accepts.
type RunReportCommand struct {
	Report string
}

// NewRunReportCall binds one report run to the resolver that answers for it.
// It holds no dependency: a report names no record at all.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRunReportCall(cmd RunReportCommand) GovernedCall {
	return bind[RunReportCommand](runReportResolver{}, cmd)
}

type runReportResolver struct{}

// Subject names NO record, and that is the honest answer rather than a gap: a
// report is an aggregate over rows the caller's own scope already bounds, so
// there is no row an approval could bind to, pin, or be probed against. What
// it does supply is the KEY — the one thing that says which aggregate is being
// released — where the route walk it replaces could only offer an empty target
// with no name attached.
func (runReportResolver) Subject(_ context.Context, cmd RunReportCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("Run report %s", cmd.Report)}, nil
}

// Guards stands down: the report key's vocabulary is the engine's catalog,
// which this module is handed rather than owns (ReportRunner, tools_report.go),
// and a key outside it is refused by the engine at execution with the catalog
// in hand. Restating it here would be a second answer that drifts the moment an
// installation's catalog does.
func (runReportResolver) Guards(_ context.Context, _ RunReportCommand) error {
	return nil
}

// ComposeReportCommand is one report rendering, whichever door asked for it.
//
// The block COUNT is the whole of it. The figures are not part of the command
// and must not be: each is a citation resolved under the reading caller's own
// authority at render time, so a summary naming them would state numbers the
// approver may not be entitled to see — and would state them from the
// composer's view rather than the reader's.
type ComposeReportCommand struct {
	Blocks int
}

// NewComposeReportCall binds one report rendering to the resolver that answers
// for it. It holds no dependency: a report names no record at all.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewComposeReportCall(cmd ComposeReportCommand) GovernedCall {
	return bind[ComposeReportCommand](composeReportResolver{}, cmd)
}

type composeReportResolver struct{}

// Subject names NO record, for the reason a report run names none: the document
// is an arrangement of aggregates over rows the caller's own scope already
// bounds, and there is no row an approval could bind to or be probed against.
// What it supplies instead is the SIZE — the one thing that says how much is
// being released — where a route walk could offer only an empty target.
func (composeReportResolver) Subject(_ context.Context, cmd ComposeReportCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("Compose a report of %d block(s)", cmd.Blocks)}, nil
}

// Guards stands down: what a document may contain is the block grammar's, which
// this module is handed rather than owns, and a block outside it is refused at
// execution with the whole set in hand. Restating the grammar here would be a
// second answer that drifts the moment a block kind is added.
func (composeReportResolver) Guards(_ context.Context, _ ComposeReportCommand) error {
	return nil
}

// AnalyticsQueryCommand is one typed analytics run, whichever door asked for
// it. The ENTITY is the whole of it: the plan narrows what is counted, and the
// engine — not this seam — owns what a population admits.
type AnalyticsQueryCommand struct {
	Entity string
}

// NewAnalyticsQueryCall binds one analytics run to the resolver that answers
// for it. It holds no dependency: an aggregate names no record at all.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewAnalyticsQueryCall(cmd AnalyticsQueryCommand) GovernedCall {
	return bind[AnalyticsQueryCommand](analyticsQueryResolver{}, cmd)
}

type analyticsQueryResolver struct{}

// Subject names NO record, for runReportResolver's reason: an aggregate over
// rows the caller's own scope bounds has no row an approval could bind to.
// The population name is the one fact that says what is being released.
func (analyticsQueryResolver) Subject(_ context.Context, cmd AnalyticsQueryCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("Run an analytics query over %s", cmd.Entity)}, nil
}

// Guards stands down, for runReportResolver's reason: the vocabulary is the
// engine's derived schema, and a name outside it is refused at execution with
// the allowed set in hand.
func (analyticsQueryResolver) Guards(_ context.Context, _ AnalyticsQueryCommand) error {
	return nil
}
