// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The seat a lane test reads the page as.
//
// Assemble admits a member before it reads anything, so a lane fixture handing
// it context.Background() is asking the page a question no caller can ask. The
// lanes themselves have always gated — several call auth.RequireHuman — and
// those suites bound a principal for that reason; the ones that did not were
// exercising lanes that happen not to ask, which is a property of the lane and
// not of the page.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pageReader is a seated human with workspace-wide row scope: the widest
// ordinary reader, so a lane assertion fails on the lane's own rule rather
// than on a scope the fixture chose.
func pageReader() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-0000000000fe"),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// TestThePageRefusesACallerWithNoSeat holds Assemble's own admission.
//
// Every lane gates for itself and a refused lane is omitted and named, so a
// caller with no principal at all used to get a page: fourteen lanes each
// declining to answer, rendered as a day with nothing in it. That is the
// shape a clear morning has, which is why the absence had to become a
// refusal rather than an empty answer.
func TestThePageRefusesACallerWithNoSeat(t *testing.T) {
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	// The control: a seated reader is answered, so the refusal below is the
	// missing seat and not a service this fixture failed to wire.
	if _, err := svc.Assemble(pageReader()); err != nil {
		t.Fatalf("a seated reader was refused the page: %v — the fixture has to answer "+
			"somebody before its refusal of nobody means anything", err)
	}

	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Error("a caller carrying no principal was served the attention page — every lane " +
			"declining separately renders as a clear day, which is what a reader with " +
			"nothing to do also sees")
	}
}
