// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A decision behind an audit line, alone or grouped, is not drawn twice. A
// decision no line stands for is drawn once.
func TestADecisionIsDrawnOnceAcrossBothSources(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.October, 20, 9, 0, 0, 0, time.UTC)
	applied, grouped, alone := ids.NewV7(), ids.NewV7(), ids.NewV7()
	drawn := []crmcontracts.MagicLine{
		{Id: openapi_types.UUID(ids.NewV7()), ReceiptIds: &[]openapi_types.UUID{openapi_types.UUID(applied)}},
		{Id: openapi_types.UUID(ids.NewV7()), ReceiptIds: &[]openapi_types.UUID{
			openapi_types.UUID(ids.NewV7()), openapi_types.UUID(grouped),
		}},
	}
	decided := []DecidedReceipt{
		{ID: applied, Kind: "advance_deal", Summary: "Moved the deal", OccurredAt: at},
		{ID: grouped, Kind: "close_date_correction", OccurredAt: at},
		{ID: alone, Kind: "send_email", Summary: "Sent the follow-up", OccurredAt: at, TargetType: "contact", TargetID: ids.NewV7()},
		{ID: alone, Kind: "send_email", Summary: "Sent the follow-up", OccurredAt: at},
	}
	lines := decidedLines(drawn, decided)
	if len(lines) != 1 || lines[0].Id != openapi_types.UUID(alone) {
		t.Fatalf("drew %+v, want the one decision no line stands for, once", lines)
	}
	line := lines[0]
	if line.Lane != crmcontracts.MagicLineLaneMagicLaneDone || line.Summary.Key != "magic.action.decided_summary" ||
		(*line.Summary.Values)["summary"] != "Sent the follow-up" || line.Undo != nil {
		t.Fatalf("the decision line reads %+v, want a done line with its summary and no Undo", line)
	}
	if line.ReceiptIds == nil || len(*line.ReceiptIds) != 1 || (*line.ReceiptIds)[0] != openapi_types.UUID(alone) {
		t.Fatalf("the decision line names receipts %v, want its own id", line.ReceiptIds)
	}
	if line.Entity == nil || line.Entity.Type != "contact" {
		t.Fatalf("the decision line names %+v, want its target", line.Entity)
	}
}

func TestADecisionWithoutASummaryIsNamedByItsKind(t *testing.T) {
	t.Parallel()
	lines := decidedLines(nil, []DecidedReceipt{{ID: ids.NewV7(), Kind: "send_email"}})
	if len(lines) != 1 || lines[0].Summary.Key != "magic.action.decided_kind" || (*lines[0].Summary.Values)["kind"] != "send_email" {
		t.Fatalf("drew %+v, want the kind sentence", lines)
	}
}

// decisionSource answers the newest `scan` of its receipts, as the approvals
// read does, and counts how wide it was asked.
type decisionSource struct {
	receipts []DecidedReceipt
	widest   int
}

func (d *decisionSource) DecidedSince(_ context.Context, _ time.Time, scan int) ([]DecidedReceipt, bool, error) {
	d.widest = max(d.widest, scan)
	return d.receipts[:min(scan, len(d.receipts))], scan < len(d.receipts), nil
}

func decisions(n int) []DecidedReceipt {
	out := make([]DecidedReceipt, n)
	for i := range out {
		out[i] = DecidedReceipt{ID: ids.NewV7(), Kind: "advance_deal", Approval: true}
	}
	return out
}

// One grouped audit line stands for the newest `covered` decisions.
func groupedLine(receipts []DecidedReceipt, covered int) []crmcontracts.MagicLine {
	held := make([]openapi_types.UUID, 0, covered)
	for _, r := range receipts[:covered] {
		held = append(held, openapi_types.UUID(r.ID))
	}
	return []crmcontracts.MagicLine{{Id: openapi_types.UUID(ids.NewV7()), ReceiptIds: &held, Count: &covered}}
}

func TestADecisionOlderThanTheOverlapIsStillDrawn(t *testing.T) {
	t.Parallel()
	source := &decisionSource{receipts: decisions(101)}
	svc := (&Service{}).WithDecidedReceipts(source)
	lines, floor, err := svc.decidedDone(context.Background(), groupedLine(source.receipts, 100), time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Id != openapi_types.UUID(source.receipts[100].ID) || floor {
		t.Fatalf("drew %d lines (floor=%t), want the 101st decision and a complete lane", len(lines), floor)
	}
}

func TestADecisionSourceExactlyAtThePageIsComplete(t *testing.T) {
	t.Parallel()
	source := &decisionSource{receipts: decisions(100)}
	svc := (&Service{}).WithDecidedReceipts(source)
	lines, floor, err := svc.decidedDone(context.Background(), groupedLine(source.receipts, 100), time.Time{}, 100)
	if err != nil || len(lines) != 0 || floor {
		t.Fatalf("drew %d lines (floor=%t, err=%v), want none and a complete lane", len(lines), floor, err)
	}
}

func TestADecisionSourceStillFullAtTheCapMakesTheLaneAFloor(t *testing.T) {
	t.Parallel()
	source := &decisionSource{receipts: decisions(readCap + 1)}
	svc := (&Service{}).WithDecidedReceipts(source)
	_, floor, err := svc.decidedDone(context.Background(), groupedLine(source.receipts, readCap), time.Time{}, 100)
	if err != nil || !floor || source.widest != readCap {
		t.Fatalf("floor=%t widest=%d err=%v, want a floor after reading to %d", floor, source.widest, err, readCap)
	}
}

func TestADecisionLineNamesItsTargetAndItsApproval(t *testing.T) {
	t.Parallel()
	label := "PIM Rollout"
	lines := decidedLines(nil, []DecidedReceipt{{
		ID: ids.NewV7(), Kind: "send_email", Summary: "Sent the follow-up",
		TargetType: "deal", TargetID: ids.NewV7(), TargetLabel: &label, Approval: true,
	}})
	if len(lines) != 1 || lines[0].Entity == nil || lines[0].Entity.Label == nil || *lines[0].Entity.Label != label ||
		lines[0].ApprovalKind == nil || *lines[0].ApprovalKind != "send_email" {
		t.Fatalf("drew %+v, want the staged target name and the approval it opens", lines)
	}
}
