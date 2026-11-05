// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestConfirmationRequiresAQuestionRatherThanAnUnexecutedMutation(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"question":" "}`, `{"to_stage_id":"destination"}`, `{"question":"Ready?","note":"Different question"}`, `{"question":"Ready?","secret":"hidden"}`} {
		approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
		_, err := ApplyActions(context.Background(), Executors{Approvals: approvals}, workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionEmitFlowEvent, Target: datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}, Args: json.RawMessage(raw)}}})
		if !errors.Is(err, apperrors.ErrInvalidArgument) || len(approvals.calls) != 0 {
			t.Errorf("%s staged without a reviewable question: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"question":"Ready?","reason":"Account review"}`, `{"note":"Ready?","reason":"Account review"}`} {
		got, err := ReadConfirmationRequest(json.RawMessage(raw))
		if err != nil || got.Question != "Ready?" || got.Reason != "Account review" {
			t.Errorf("lost confirmation context: %+v / %v", got, err)
		}
	}
}
