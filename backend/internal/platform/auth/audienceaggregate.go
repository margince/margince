// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The audience rule for a reader that COUNTS messages rather than showing them.
//
// A held message is not only unreadable — it must not be visible in the shape
// of an aggregate either. A relationship-strength score that counts a founder's
// correspondence with their lawyer tells every colleague the relationship is
// strong; a last-activity timestamp that moves when a private message arrives
// tells them when it arrived. Neither shows a word of the message, and both
// disclose it.
//
// Spelled once here rather than per module because four readers ask the same
// question — relationship strength, deal health, the meeting brief and the
// weekly digest — and a fifth will be written by somebody who greps for it. The
// graph projection asked it first and carried its own copy; that copy moved
// here rather than being joined by a second.

// AudienceWorkspaceOnly filters an activity aliased by name down to the
// activities the whole workspace may see, including a task's source audience.
//
// It takes no placeholder, so it renumbers nothing at a call site. A caller
// concatenates it into a `WHERE` clause and the surrounding arguments keep
// their positions.
//
// It is not the reader's own audience. Colleagues who did not receive the mail
// read an aggregate too. A per-caller answer would give two contacts different
// numbers for the same account, which reads as a bug and discloses the
// difference between them. An aggregate asks "may everyone see this", and the
// answer is the same for everybody.
func AudienceWorkspaceOnly(alias string) string {
	return " AND " + alias + ".audience = 'workspace' AND (" + alias + ".kind <> 'task' OR " + alias + ".source_activity_id IS NULL AND coalesce(" + alias + ".source_system, '') <> '" + provenance.LeadRepliedSource + "' OR EXISTS (SELECT 1 FROM activity follow_up_source WHERE follow_up_source.id = " + provenance.TaskSourceActivitySQL(alias) + " AND follow_up_source.audience = 'workspace' AND follow_up_source.restricted_at IS NULL))"
}

// ReaderlessAudience narrows the system principal to the rows every member may
// read, for a per-reader date read by a job. The content gate admits every
// audience for that principal, and no reader stands behind its answer.
func ReaderlessAudience(ctx context.Context, alias string) string {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalSystem {
		return AudienceWorkspaceOnly(alias)
	}
	return ""
}
