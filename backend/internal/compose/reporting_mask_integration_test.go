// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingMeetingFieldRestrictionsReachFrozenChartsAndEvidence(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"sdr_outcomes"}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Meeting review", Audience: "private", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "meeting-mask")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:meeting-mask")); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("publication: %+v %v", completed, err)
	}
	original, err := f.service.GetEdition(f.human, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if original.Evaluation.Metrics[0].Value == nil || *original.Evaluation.Metrics[0].Value != 12 {
		t.Fatal("fixture did not publish twelve meetings")
	}
	for _, field := range []string{"meeting_status", "scheduled_start_at", "host_user_id", "subject"} {
		t.Run(field, func(t *testing.T) {
			actor, ok := principal.Actor(f.human)
			if !ok {
				t.Fatal("missing fixture principal")
			}
			actor.Permissions.RowScope = principal.RowScopeOwn
			actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "activity", Field: field, Condition: principal.MaskAlways}}
			masked := principal.WithActor(f.human, actor)
			edition, err := f.service.GetEdition(masked, ids.UUID(original.Id))
			if err != nil {
				t.Fatal(err)
			}
			metric := edition.Evaluation.Metrics[0]
			if !edition.Withheld || !metric.Coverage.Withheld || (metric.Value != nil && *metric.Value != 0) {
				t.Fatalf("frozen meeting remained visible under %s: %+v", field, metric)
			}
			for _, chart := range edition.Evaluation.Charts {
				for _, point := range chart.Points {
					if point.Value != nil && *point.Value != 0 {
						t.Fatalf("restricted chart retained a value: %+v", point)
					}
				}
			}
			evidence, err := f.service.EditionEvidence(masked, ids.UUID(original.Id), "meetings_held", "interval", "", nil, nil, 100)
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.Rows) != 0 {
				t.Fatalf("restricted evidence retained %d rows", len(evidence.Rows))
			}
		})
	}
}
