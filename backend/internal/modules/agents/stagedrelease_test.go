// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// What a caller is told about a staged call must be true of it: the relay
// through decide_approval is offered to a credential that can use it, and a
// credential that cannot is sent to the contact instead.
func TestAStagedCallOffersTheRelayOnlyWhereTheCredentialCanRelease(t *testing.T) {
	args, _ := stageableToolArgs()
	for _, tc := range []struct {
		name       string
		releasable bool
	}{
		{"a change the credential can undo and release", true},
		{"a change only the contact releases", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry(&recordingApprovals{releasable: tc.releasable}, nil)
			registerEveryStageableFamily(registry, localProvider{}, &recordingComms{})

			err := registry.stageRefusedCall(context.Background(), registry.tools["archive_record"], "archive_record",
				json.RawMessage(args["archive_record"]), "hash", apperrors.ErrRequiresApproval)

			var staged *workflow.StagedApprovalError
			if !errors.As(err, &staged) {
				t.Fatalf("staging answered %v, want a StagedApprovalError", err)
			}
			if staged.ReleasableByCaller != tc.releasable {
				t.Fatalf("ReleasableByCaller = %v, want %v", staged.ReleasableByCaller, tc.releasable)
			}
			text := stagedExplanation(staged)
			if got := strings.Contains(text, "decide_approval"); got != tc.releasable {
				t.Errorf("offers decide_approval = %v, want %v:\n%s", got, tc.releasable, text)
			}
			if !tc.releasable && !strings.Contains(text, "contact releases it in the CRM") {
				t.Errorf("a credential that cannot release is not sent to the contact:\n%s", text)
			}
			if !strings.Contains(text, staged.ApprovalID.String()) {
				t.Errorf("the retry id is missing:\n%s", text)
			}
		})
	}
}

// A relink is undoable by where it files, whatever the tool's static tier.
func TestReleaseUndoableByDestination(t *testing.T) {
	for _, tc := range []struct {
		tool, call          string
		undoable, isDecided bool
	}{
		{"relink_activities", `{"entity_type":"company"}`, true, true},
		{"relink_activity", `{"entity_type":"lead","activity_id":"x"}`, true, true},
		{"relink_thread", `{"entity_type":"project"}`, false, true},
		{"relink_activities", ``, false, true},
		{"advance_deal", `{"entity_type":"company"}`, false, false},
		{"update_record", `{}`, false, false},
	} {
		undoable, decided := ReleaseUndoableByDestination(tc.tool, json.RawMessage(tc.call))
		if undoable != tc.undoable || decided != tc.isDecided {
			t.Errorf("%s %s = (undoable %v, decided %v), want (%v, %v)",
				tc.tool, tc.call, undoable, decided, tc.undoable, tc.isDecided)
		}
	}
}
