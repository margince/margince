// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What draftFollowUpReply does with each answer the address resolver can give.
//
// The fallback to the task proposal must cover exactly one case — this thread
// has no counterparty. An earlier cut treated EVERY error as that case, so a
// denied read or a database failure was reported as a successful nightly pass
// that chose the other proposal, and the draft was never retried.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addressStub answers ReplyAddress with whatever the case under test needs.
type addressStub struct {
	address string
	err     error
}

func (s addressStub) ReplyAddress(context.Context, ids.UUID) (string, error) {
	return s.address, s.err
}

func (s addressStub) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "Re: something", "some words", nil
}

func TestOnlyAMissingCounterpartyFallsBackToTheTaskProposal(t *testing.T) {
	proposal := deals.FollowUpProposal{EvidenceActivityID: ids.From[ids.ActivityKind](ids.NewV7())}
	cases := []struct {
		name       string
		stub       addressStub
		wantDraft  bool
		wantFailed bool
	}{
		{
			name:      "a thread with a counterparty is drafted",
			stub:      addressStub{address: "anna@example.test"},
			wantDraft: true,
		},
		{
			name: "a thread carrying no address falls back",
			stub: addressStub{err: &activities.NoReplyAddressError{}},
		},
		{
			name: "an empty address with no error falls back",
			stub: addressStub{address: ""},
		},
		{
			name:       "a denied read is a failure, not a fallback",
			stub:       addressStub{err: apperrors.ErrPermissionDenied},
			wantFailed: true,
		},
		{
			name:       "a database failure is a failure, not a fallback",
			stub:       addressStub{err: errors.New("connection reset")},
			wantFailed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, drafted, err := draftFollowUpReply(context.Background(), tc.stub, proposal)
			if tc.wantFailed {
				if err == nil {
					t.Fatal("the pass reported success — a real failure reported as a " +
						"fallback is a nightly run that hides it and never retries")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected failure: %v", err)
			}
			if drafted != tc.wantDraft {
				t.Errorf("drafted = %v, want %v", drafted, tc.wantDraft)
			}
		})
	}
}
