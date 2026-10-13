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
		{"relink_activities", `{"entity_type":"project"}`, true, true},
		{"relink_activities", `{"entity_type":"workspace"}`, false, true},
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
// A project is the deliberate difference: its tier waits for a contact, and the
// filing is still undoable because a member can take it back.
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
			tier := spec.TierResolver(mcp.TierResolverInput{Args: call})
			if wantTier := map[string]mcp.RiskTier{"project": mcp.TierConfirmationRequired, "company": mcp.TierAutoExecute}[entity]; tier != wantTier {
				t.Errorf("%s onto a %s resolves tier %v, want %v", name, entity, tier, wantTier)
			}
			if !decided || !undoable {
				t.Errorf("%s onto a %s: classified undoable=%v decided=%v, want both true", name, entity, undoable, decided)
			}
		}
	}
}

// The thread tool never runs, at any destination and whatever the context says:
// a company, contact, deal or lead move resolves to the auto-execute tier, so
// the refusal has to live on the execution path itself, not only where a call is
// staged or an approval redeemed.
func TestAThreadRelinkIsRefusedOnTheExecutionPathAtEveryDestination(t *testing.T) {
	for _, entity := range []string{"company", "contact", "deal", "lead", "project"} {
		for name, ctx := range map[string]context.Context{
			"a direct call":       context.Background(),
			"a redeemed approval": withApprovalRedeemed(context.Background(), 0, false),
		} {
			relinker := &recordingRelinker{}
			_, err := relinkThread{relinker: relinker}.Handle(ctx, json.RawMessage(
				`{"thread_key":"thread:x","entity_type":"`+entity+`","entity_id":"`+ids.NewV7().String()+`"}`,
			))

			var bad *BadArgsError
			if !errors.As(err, &bad) || !strings.Contains(bad.Guidance, "relink_activities") {
				t.Errorf("%s onto a %s → %v, want the thread-key refusal", name, entity, err)
			}
			if relinker.entityType != "" {
				t.Errorf("%s onto a %s moved the thread", name, entity)
			}
		}
	}
}
