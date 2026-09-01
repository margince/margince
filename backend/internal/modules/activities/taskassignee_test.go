// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who a new task belongs to.
//
// "Mine" became exact, which is only safe because a task somebody writes for
// themselves is owned as it is written. These are the cases that hold that up:
// lose the first and a rep's own to-do vanishes from their day, lose the second
// and every automation's work lands on whoever the job ran as.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func humanWriting(user ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:" + user.String(),
		UserID: user,
	})
}

// A rep who writes themselves a task owns it, without naming themselves.
func TestASelfWrittenTaskBelongsToItsAuthor(t *testing.T) {
	author := ids.NewV7()

	got := taskAssignee(humanWriting(author), LogActivityInput{Kind: "task"})

	if got == nil {
		t.Fatal("a task a rep wrote for themselves came out belonging to nobody")
	}
	if got.UUID != author {
		t.Fatalf("the task belongs to %v, wanted its author %v", got.UUID, author)
	}
}

// A named assignee is obeyed. Handing a task to a colleague must not silently
// become keeping it.
func TestANamedAssigneeIsKept(t *testing.T) {
	author, colleague := ids.NewV7(), ids.From[ids.UserKind](ids.NewV7())

	got := taskAssignee(humanWriting(author), LogActivityInput{Kind: "task", AssigneeID: &colleague})

	if got == nil || got.UUID != colleague.UUID {
		t.Fatalf("the task went to %v, wanted the named colleague %v", got, colleague)
	}
}

// A system principal owns nothing. An automation's follow-up belongs to the
// record it was minted for, or to nobody until routing says otherwise — never
// to the job that happened to run.
func TestASystemWrittenTaskBelongsToNobody(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "system:time-scan",
	})

	if got := taskAssignee(ctx, LogActivityInput{Kind: "task"}); got != nil {
		t.Fatalf("a system-written task was assigned to %v", got)
	}
}

// Only tasks. An email or a note has no assignee, and stamping one would put a
// column on a row whose CHECK constraint refuses it.
func TestOnlyATaskGetsAnAssignee(t *testing.T) {
	author := ids.NewV7()

	for _, kind := range []string{"email", "call", "note", "meeting"} {
		if got := taskAssignee(humanWriting(author), LogActivityInput{Kind: kind}); got != nil {
			t.Fatalf("a %s was assigned to %v", kind, got)
		}
	}
}
