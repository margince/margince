// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// DraftReportCommand is one draft saved on a report, whichever door asked for
// it. The charts are not part of it. A draft changes nothing a reader sees, and
// the human who publishes it reads the draft itself.
type DraftReportCommand struct {
	ReportID ids.UUID
}

// NewDraftReportCall binds one report draft to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDraftReportCall(language baselanguage.Resolver, cmd DraftReportCommand) GovernedCall {
	return bind[DraftReportCommand](draftReportResolver{language: language}, cmd)
}

type draftReportResolver struct {
	language baselanguage.Resolver
}

// Subject names the report the draft is saved on.
func (r draftReportResolver) Subject(ctx context.Context, cmd DraftReportCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: reportDefinitionRecordType,
		TargetID:   cmd.ReportID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).draftReport, cmd.ReportID),
	}, nil
}

// Guards stands down: who may edit the report is the reporting service's
// question, asked where the draft is saved.
func (draftReportResolver) Guards(_ context.Context, _ DraftReportCommand) error {
	return nil
}

const reportDefinitionRecordType = "report_definition"
