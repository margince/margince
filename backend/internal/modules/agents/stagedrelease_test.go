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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
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
		{"relink_thread", `{"entity_type":"company"}`, false, false},
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

// The set the classifier judges by destination is derived from the tools
// themselves: each name is registered and its tier resolver answers what the
// classifier does for a project and for a company, so a change to either shows up here.
func TestEveryDestinationToolIsClassifiedByItsOwnTierResolver(t *testing.T) {
	registry := NewRegistry(&recordingApprovals{}, nil)
	registerEveryStageableFamily(registry, localProvider{}, &recordingComms{})
	if len(relinkDestinationTools) == 0 {
		t.Fatal("no destination tool is named — this compared nothing")
	}
	for name := range relinkDestinationTools {
		spec, registered := registry.Spec(name)
		if !registered || spec.TierResolver == nil {
			t.Errorf("%s is judged by destination but is not a registered dynamic tool", name)
			continue
		}
		for _, entity := range []string{"project", "company"} {
			call := json.RawMessage(`{"entity_type":"` + entity + `"}`)
			undoable, decided := ReleaseUndoableByDestination(name, call)
			if want := spec.TierResolver(mcp.TierResolverInput{Args: call}) == mcp.TierAutoExecute; !decided || undoable != want {
				t.Errorf("%s onto a %s: classified undoable=%v decided=%v, its tier says undoable=%v", name, entity, undoable, decided, want)
			}
		}
	}
}

// A released approval for a thread names a key, so the tool never runs one —
// including a proposal staged before thread moves stopped staging.
func TestARedeemedThreadRelinkIsRefusedBeforeItMovesAnything(t *testing.T) {
	relinker := &recordingRelinker{}
	ctx := withApprovalRedeemed(context.Background(), 0, false)

	_, err := relinkThread{relinker: relinker}.Handle(ctx, json.RawMessage(
		`{"thread_key":"thread:x","entity_type":"company","entity_id":"`+ids.NewV7().String()+`"}`,
	))

	var bad *BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Guidance, "relink_activities") {
		t.Fatalf("a redeemed thread move → %v, want the thread-key refusal", err)
	}
	if relinker.entityType != "" {
		t.Error("the thread was moved under a released approval")
	}
}
