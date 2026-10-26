// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The done lane's second source: decisions the system took for the reader.
//
// The audit rows and the system's decisions overlap. An applied stage move is
// both an audit row and an approval. A close-date correction's receipt has
// its audit row's id. Each line carries the receipt ids it stands for, so a
// decision already behind a line is not drawn twice.

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DecidedReceipts reads what the system decided or applied in the window,
// under the approvals' own reader rule.
type DecidedReceipts interface {
	// DecidedSince answers at most limit receipts, newest first, and whether
	// the source holds more than it answered.
	DecidedSince(ctx context.Context, since time.Time, limit int) ([]DecidedReceipt, bool, error)
}

// DecidedReceipt is one system decision, by its canonical receipt id.
type DecidedReceipt struct {
	ID         ids.UUID
	Kind       string
	Summary    string
	OccurredAt time.Time
	// TargetType is empty when the decision named no record.
	TargetType string
	TargetID   ids.UUID
	// TargetLabel is the name the decision was taken against, frozen when it
	// was staged: the name the needs-you lane showed the same reader.
	TargetLabel *string
	// Approval says the receipt is an approval, whose own page is where the
	// decision is revisited.
	Approval bool
}

// WithDecidedReceipts binds the second source. Unbound, the lane holds the
// audit rows alone.
func (s *Service) WithDecidedReceipts(d DecidedReceipts) *Service {
	s.decided = d
	return s
}

// decidedDone reads the decisions and keeps those no drawn line stands for.
// A seat that may not read approvals keeps its audit lines; the needs-you
// lane already reports that source as withheld.
func (s *Service) decidedDone(
	ctx context.Context, drawn []crmcontracts.MagicLine, from time.Time, limit int,
) (lines []crmcontracts.MagicLine, floor bool, err error) {
	if s.decided == nil {
		return []crmcontracts.MagicLine{}, false, nil
	}
	// Refilled while overlap with the drawn lines leaves the page short and
	// the source has more. Stopped at readCap, the lane's total is a floor.
	for scan := limit; ; scan = min(scan*4, readCap) {
		decided, more, err := s.decided.DecidedSince(ctx, from, scan)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []crmcontracts.MagicLine{}, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		lines := decidedLines(drawn, decided)
		if !more || len(lines) >= limit {
			return lines, false, nil
		}
		if scan >= readCap {
			return lines, true, nil
		}
	}
}

func decidedLines(drawn []crmcontracts.MagicLine, decided []DecidedReceipt) []crmcontracts.MagicLine {
	shown := map[openapi_types.UUID]bool{}
	for _, line := range drawn {
		if line.ReceiptIds != nil {
			for _, id := range *line.ReceiptIds {
				shown[id] = true
			}
		}
	}
	out := make([]crmcontracts.MagicLine, 0, len(decided))
	for _, receipt := range decided {
		id := openapi_types.UUID(receipt.ID)
		if shown[id] {
			continue
		}
		shown[id] = true
		out = append(out, decidedLine(receipt))
	}
	return out
}

// decidedLine dresses one decision. It offers no Undo. The record an
// approval named can revisit it, and a correction with a way back is always
// an audit line already.
func decidedLine(receipt DecidedReceipt) crmcontracts.MagicLine {
	values := map[string]string{"kind": receipt.Kind}
	sentence := "magic.action.decided_kind"
	if summary := approvals.SanitizeSummary(receipt.Summary); summary != "" {
		sentence = "magic.action.decided_summary"
		values["summary"] = summary
	}
	line := crmcontracts.MagicLine{
		Id:         openapi_types.UUID(receipt.ID),
		ReceiptIds: new([]openapi_types.UUID{openapi_types.UUID(receipt.ID)}),
		OccurredAt: receipt.OccurredAt,
		Lane:       crmcontracts.MagicLineLaneMagicLaneDone,
		Summary:    crmcontracts.MagicSentence{Key: sentence, Values: &values},
		Actor: crmcontracts.MagicActor{
			Type:  crmcontracts.MagicActorTypeMagicActorSystem,
			Id:    systemProposer,
			Label: &crmcontracts.MagicSentence{Key: "magic.by.system"},
		},
	}
	if receipt.TargetType != "" && !receipt.TargetID.IsZero() {
		line.Entity = &crmcontracts.MagicEntityRef{
			Type: receipt.TargetType, Id: openapi_types.UUID(receipt.TargetID), Label: receipt.TargetLabel,
		}
	}
	if receipt.Approval {
		line.ApprovalKind = &receipt.Kind
	}
	return line
}
