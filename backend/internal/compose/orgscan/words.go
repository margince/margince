// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// The recent exchanges' own words, read as the viewer.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// scanMessages bounds how many exchanges the model reads. Past twenty the
	// prompt is a mailbox, not a conversation, and the findings that matter
	// are in the newest ones anyway.
	scanMessages = 20
	// scanBodyChars bounds one body. A commitment, a question or a risk is
	// stated in a message's opening; past this is quoted history and a
	// signature, which the model would otherwise cite as if it were new.
	scanBodyChars = 1200
	// scanWindowDays is how far back an exchange still needs a person. A
	// question from a year ago that nobody answered has been answered by time.
	scanWindowDays = 180
)

// scopeAll is the predicate a reader with no row scope gets.
const scopeAll = "TRUE"

// readWords reads the newest exchanges on the account, oldest first, with
// their own words.
//
// Gated on ActivityContentClause, not the discover clause: the bodies go into
// a prompt, and a reader allowed to know a message exists is not thereby
// allowed to have it read for them. The activity grant is REFUSED rather than
// softened, for the reason the role proposals give — an account whose
// messages this reader may not read is not a quiet account, and a scan that
// read as one would be advising from a fact about their permissions.
//
// Reachability is activities.OrgLinkedActivityExists, the walk every reader
// of the account's timeline uses, so a message linked through one of the
// account's deals counts as the account's own correspondence.
func readWords(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time,
) ([]MessageIn, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	sincePos := arg(now.AddDate(0, 0, -scanWindowDays))
	charsPos := arg(scanBodyChars)
	capPos := arg(scanMessages)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.kind, coalesce(a.direction, ''), coalesce(a.subject, ''), a.occurred_at,
		       coalesce(left(a.body, $%[3]d), ''),
		       greatest(coalesce(char_length(a.body), 0) - $%[3]d, 0)
		  FROM activity a
		 WHERE a.kind IN ('email','message','call','meeting') AND a.archived_at IS NULL
		   AND a.occurred_at >= $%[2]d AND (%[5]s)
		   AND %[1]s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%[4]d`,
		activities.OrgLinkedActivityExists(orgPos), sincePos, charsPos, capPos, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("orgscan: reading the account's exchanges: %w", err)
	}
	defer rows.Close()
	var out []MessageIn
	for rows.Next() {
		var message MessageIn
		if err := rows.Scan(&message.ID, &message.Kind, &message.Direction, &message.Subject,
			&message.At, &message.Text, &message.UnreadChars); err != nil {
			return nil, err
		}
		message.At = message.At.UTC()
		out = append(out, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Newest first is the read's order, because the cap keeps the newest;
	// oldest first is the model's, because a conversation is read forwards.
	slices.Reverse(out)
	return out, nil
}
