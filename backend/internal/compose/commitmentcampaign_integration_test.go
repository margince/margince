// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/schema"
)

func TestOneVerifiedPromiseKeepsEveryRecipientsClaim(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		confidence     float64
		refreshPending bool
		laterAction    string
	}{{"automatic task", 0.95, false, ""}, {"approved task", 0.75, false, ""}, {"pending recipients change", 0.75, true, ""}, {"human edited task", 0.95, false, "edit"}, {"completed before review", 0.75, false, "complete"}, {"rejected recipients", 0.75, false, "reject"}} {
		t.Run(scenario.name, func(t *testing.T) {
			confidence := scenario.confidence
			e := setupCompletionTranscript(t, "Priya Raman: Ines, Morgan and Pat, I will send the recording to all three of you.")
			e.GrantRole(t, e.Rep2, "rep")
			refs := []string{}
			for _, name := range []string{"Ines Huber", "Morgan Recipient", "Pat Buyer"} {
				contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, OwnerID: new(ids.From[ids.UserKind](e.Rep2))})
				if err != nil {
					t.Fatal(err)
				}
				id := ids.UUID(contact.Id)
				e.linkTranscriptContact(t, id)
				refs = append(refs, "contact:"+id.String())
			}
			raw, err := json.Marshal(map[string][]proposedStep{"proposals": {{
				Summary: "Send the recording", Owner: "Priya Raman", PromiserRef: "seat:" + e.Rep2.String(), RecipientRefs: refs[:2],
				SourceLines: []int{1}, Confidence: schema.Confidence(confidence), Meaning: openCommitmentMeaning("", ""),
			}}})
			if err != nil {
				t.Fatal(err)
			}
			reading := e.readAt(t, cannedBrain{reply: string(raw)}, func() time.Time { return completionReadTime })
			if confidence < CommitmentTaskConfidence {
				if len(reading.ProposalIDs) != 1 {
					t.Fatalf("got %d proposals, want one", len(reading.ProposalIDs))
				}
				if scenario.refreshPending {
					reading, raw = e.refreshCampaignRecipients(t, reading.ProposalIDs[0], raw, refs)
				}
				e.approveCampaignRecipients(t, reading.ProposalIDs[0], scenario.refreshPending)
			}
			if e.taskCount(t) != 1 {
				t.Fatal("one deliverable must create one task")
			}
			initialRecipients := 2
			if scenario.refreshPending {
				initialRecipients = 3
			}
			if count := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1 AND task_activity_id IS NOT NULL`, e.activity); count != initialRecipients {
				t.Fatalf("linked recipient claims=%d, want %d", count, initialRecipients)
			}
			if count := e.WsCount(t, `SELECT count(DISTINCT evidence_fingerprint) FROM conversation_claim WHERE source_activity_id=$1`, e.activity); count != 1 {
				t.Fatal("recipients lost their shared promise identity")
			}
			e.readAt(t, cannedBrain{reply: string(raw)}, func() time.Time { return completionReadTime })
			if e.taskCount(t) != 1 || e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, e.activity) != initialRecipients {
				t.Fatal("rereading duplicated the task or its recipients")
			}
			if scenario.laterAction == "edit" {
				updateCampaignTask(t, e, activities.UpdateActivityInput{Subject: new("Human wording for the recording")})
			}
			updated := campaignReadingWithRecipients(t, raw, refs)
			later := e.readAt(t, cannedBrain{reply: string(updated)}, func() time.Time { return completionReadTime })
			want := 3
			if (confidence < CommitmentTaskConfidence && !scenario.refreshPending) || scenario.laterAction == "edit" {
				want = 2
			}
			if count := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1 AND task_activity_id IS NOT NULL`, e.activity); count != want {
				t.Fatalf("recipient claims after later reading=%d, want %d", count, want)
			}
			if e.taskCount(t) != 1 {
				t.Fatal("later recipient created a second task")
			}
			if want == 2 {
				if len(later.ProposalIDs) != 1 {
					t.Fatalf("late recipient needs one review, got %d", len(later.ProposalIDs))
				}
				expected := resolveLateRecipientReview(t, e, later.ProposalIDs[0], scenario.laterAction, updated)
				if count := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1 AND task_activity_id IS NOT NULL`, e.activity); count != expected {
					t.Fatalf("linked recipients=%d, want %d", count, expected)
				}
			}
		})
	}
}

func campaignReadingWithRecipients(t *testing.T, raw []byte, refs []string) []byte {
	t.Helper()
	var later transcriptPayload
	if err := json.Unmarshal(raw, &later); err != nil {
		t.Fatal(err)
	}
	(*later.Proposals)[0].RecipientRefs = refs
	updated, err := json.Marshal(later)
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func acceptLateRecipientReview(t *testing.T, e *transcriptEnv, id ids.UUID) {
	t.Helper()
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	before := e.wsString(t, `SELECT row_to_json(a)::text FROM activity a WHERE kind='task'`)
	approval, err := e.svc.GetWire(reader, ids.From[ids.ApprovalKind](id))
	if err != nil {
		t.Fatal(err)
	}
	if approval.Preview == nil || approval.Preview.State != previewReadyState || approval.Preview.Effects == nil || !slices.Contains(*approval.Preview.Effects, "attach_task_recipients") {
		t.Fatalf("recipient review effects=%+v", approval.Preview)
	}
	if _, err := e.svc.Decide(reader, ids.From[ids.ApprovalKind](id), true, nil); err != nil {
		t.Fatal(err)
	}
	if e.taskCount(t) != 1 || e.wsString(t, `SELECT row_to_json(a)::text FROM activity a WHERE kind='task'`) != before {
		t.Fatal("linking a recipient changed the accepted task")
	}
}

func updateCampaignTask(t *testing.T, e *transcriptEnv, patch activities.UpdateActivityInput) {
	t.Helper()
	id, err := ids.Parse(e.wsString(t, `SELECT id::text FROM activity WHERE kind='task'`))
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	task, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	patch.IfVersion = task.Version
	if _, err := e.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](id), patch); err != nil {
		t.Fatal(err)
	}
}

func resolveLateRecipientReview(t *testing.T, e *transcriptEnv, id ids.UUID, action string, reply []byte) int {
	t.Helper()
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	expected := 3
	switch action {
	case "complete":
		updateCampaignTask(t, e, activities.UpdateActivityInput{IsDone: new(true)})
		proposal, err := e.svc.GetWire(reader, ids.From[ids.ApprovalKind](id))
		if err != nil {
			t.Fatal(err)
		}
		if proposal.Preview == nil || proposal.Preview.State != previewStaleState {
			t.Fatal("completed task still has an actionable recipient review")
		}
		if _, err := e.svc.Decide(reader, ids.From[ids.ApprovalKind](id), true, nil); err == nil {
			t.Fatal("recipient attached to completed task")
		}
		expected = 2
	case "reject":
		if _, err := e.svc.Decide(reader, ids.From[ids.ApprovalKind](id), false, nil); err != nil {
			t.Fatal(err)
		}
		expected = 2
	default:
		acceptLateRecipientReview(t, e, id)
	}
	reread := e.readAt(t, cannedBrain{reply: string(reply)}, func() time.Time { return completionReadTime })
	if len(reread.ProposalIDs) != 0 || e.taskCount(t) != 1 {
		t.Fatal("replay recreated decided or completed work")
	}
	return expected
}

func (e *transcriptEnv) approveCampaignRecipients(t *testing.T, proposal ids.UUID, refreshPending bool) {
	t.Helper()
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	preview, err := e.svc.GetWire(reader, ids.From[ids.ApprovalKind](proposal))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Preview == nil || preview.Preview.State != previewReadyState {
		t.Fatalf("commitment preview is not ready: %+v", preview.Preview)
	}
	recipientNames := ""
	for _, change := range preview.Preview.Changes {
		if change.Field == "recipients" {
			recipientNames = change.After.Text
		}
	}
	if !strings.Contains(recipientNames, "Ines Huber") || !strings.Contains(recipientNames, "Morgan Recipient") {
		t.Fatalf("preview omitted recipients: %q", recipientNames)
	}
	if refreshPending && !strings.Contains(recipientNames, "Pat Buyer") {
		t.Fatal("replacement preview omitted the new recipient")
	}
	if _, err := e.svc.Decide(reader, ids.From[ids.ApprovalKind](proposal), true, nil); err != nil {
		t.Fatal(err)
	}
}

func (e *transcriptEnv) refreshCampaignRecipients(t *testing.T, old ids.UUID, raw []byte, refs []string) (activities.TranscriptRead, []byte) {
	t.Helper()
	raw = campaignReadingWithRecipients(t, raw, refs)
	reading := e.readAt(t, cannedBrain{reply: string(raw)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 1 || reading.ProposalIDs[0] == old {
		t.Fatal("changed recipients need a replacement proposal")
	}
	if e.taskCount(t) != 0 {
		t.Fatal("refresh must still wait for approval")
	}
	if e.WsCount(t, `SELECT count(*) FROM approval WHERE id=$1 AND status='expired'`, old) != 1 {
		t.Fatal("previous proposal still actionable")
	}
	return reading, raw
}
