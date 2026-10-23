// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
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
	if line.Entity == nil || line.Entity.Type != "contact" || line.Entity.Label != nil {
		t.Fatalf("the decision line names %+v, want its target without a label this read never scoped", line.Entity)
	}
}

func TestADecisionWithoutASummaryIsNamedByItsKind(t *testing.T) {
	t.Parallel()
	lines := decidedLines(nil, []DecidedReceipt{{ID: ids.NewV7(), Kind: "send_email"}})
	if len(lines) != 1 || lines[0].Summary.Key != "magic.action.decided_kind" || (*lines[0].Summary.Values)["kind"] != "send_email" {
		t.Fatalf("drew %+v, want the kind sentence", lines)
	}
}
