// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type claimsOnTasks struct {
	claims []contacts.TaskClaim
	err    error
}

func (c claimsOnTasks) ClaimsOnTasks(context.Context, []ids.UUID) ([]contacts.TaskClaim, error) {
	return c.claims, c.err
}

func claimOn(task, contact ids.UUID, status string) contacts.TaskClaim {
	return contacts.TaskClaim{Contact: contact, Task: &task, Status: status}
}

func TestOnlyAPromiseTaskReadFromAMessageIsACandidateWithItsOpenRecipients(t *testing.T) {
	promise, unsourced, hand, source := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	open, settled, cc := ids.NewV7(), ids.NewV7(), ids.NewV7()
	claims := claimsOnTasks{claims: []contacts.TaskClaim{
		claimOn(promise, open, "open"), claimOn(promise, settled, "done"), claimOn(hand, cc, "open"),
	}}
	got, err := DoneCandidates(context.Background(), claims, []PromiseTask{
		{ID: promise, System: activities.CommitmentTaskSource, Source: &source},
		{ID: unsourced, System: activities.CommitmentTaskSource},
		{ID: hand, Source: &source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskID != promise || got[0].SourceActivityID != source || !slices.Equal(got[0].Recipients, []ids.UUID{open}) {
		t.Fatalf("candidates = %+v, want the promise task with its one open recipient", got)
	}
}

func TestAReaderWhoMayNotReadTheRecipientsHasNoCandidates(t *testing.T) {
	source := ids.NewV7()
	got, err := DoneCandidates(context.Background(), claimsOnTasks{err: apperrors.ErrPermissionDenied},
		[]PromiseTask{{ID: ids.NewV7(), System: activities.CommitmentTaskSource, Source: &source}})
	if err != nil || len(got) != 0 {
		t.Fatalf("candidates = %+v, %v; want none and no error", got, err)
	}
}
