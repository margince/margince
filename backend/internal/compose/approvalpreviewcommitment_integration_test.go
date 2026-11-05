// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCommitmentPreviewWithholdsARecipientMadePrivate(t *testing.T) {
	e := setupCompletionTranscript(t, "Priya Raman: Morgan, I will send you the recording.")
	e.GrantRole(t, e.Rep2, "rep")
	recipient := e.SeedContact(t, "Morgan Recipient", &e.Rep3)
	e.linkTranscriptContact(t, recipient)
	reply, err := json.Marshal(map[string][]proposedStep{"proposals": {{Summary: "Send the recording", Owner: "Priya Raman", PromiserRef: "seat:" + e.Rep2.String(), RecipientRefs: []string{"contact:" + recipient.String()}, SourceLines: []int{1}, Confidence: 0.75, Meaning: openCommitmentMeaning("", "")}}})
	if err != nil {
		t.Fatal(err)
	}
	reading := e.readAt(t, cannedBrain{reply: string(reply)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 1 {
		t.Fatal("expected one proposed task")
	}
	id := ids.From[ids.ApprovalKind](reading.ProposalIDs[0])
	reviewer := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	before, err := e.svc.GetWire(reviewer, id)
	if err != nil {
		t.Fatal(err)
	}
	if before.Preview == nil || before.Preview.State != previewReadyState {
		t.Fatal("readable recipient needs a ready preview")
	}
	permissions := transcriptPerms
	permissions.RowScope = principal.RowScopeAll
	owner := e.As(e.Rep3, nil, permissions)
	if _, err := e.Contacts.UpdateContact(owner, ids.From[ids.ContactKind](recipient), contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetWire(reviewer, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private source approval must be hidden: %v", err)
	}
	after, err := newApprovalPreviewReader(e.DB()).read(reviewer, before)
	if err != nil {
		t.Fatal(err)
	}
	if after == nil || after.State != previewUnavailableState || len(after.Changes) != 0 {
		t.Fatalf("private recipient preview=%+v", after)
	}

	encoded, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Morgan") {
		t.Fatal("private recipient name remained in preview")
	}
	if _, err := e.svc.Decide(reviewer, id, true, nil); err == nil {
		t.Fatal("approval ignored missing recipient context")
	}
	if e.taskCount(t) != 0 {
		t.Fatal("refused approval created a task")
	}
}
