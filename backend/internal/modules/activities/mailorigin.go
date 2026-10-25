// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withMailOrigins names, on each file that hangs off a mail, the mail it came
// with: subject, date and sender, for the Documents lists.
//
// The mail is read under the activity content gate, as the email viewer reads
// it. A caller who may not read the mail learns neither subject nor sender.
// The lists admit a mail's file only past that gate, and the origin asks again.
func withMailOrigins(ctx context.Context, tx pgx.Tx, files []crmcontracts.Attachment) error {
	origins, err := mailOrigins(ctx, tx, mailsOf(files))
	if err != nil {
		return err
	}
	for i := range files {
		if files[i].EntityType == crmcontracts.AttachmentEntityTypeActivity {
			files[i].Origin = origins[ids.UUID(files[i].EntityId)]
		}
	}
	return nil
}

// mailsOf is the distinct mails the files hang off.
func mailsOf(files []crmcontracts.Attachment) []ids.UUID {
	seen := map[ids.UUID]bool{}
	var mails []ids.UUID
	for _, file := range files {
		mail := ids.UUID(file.EntityId)
		if file.EntityType == crmcontracts.AttachmentEntityTypeActivity && !seen[mail] {
			seen[mail] = true
			mails = append(mails, mail)
		}
	}
	return mails
}

// mailOrigins reads the origin of each live mail the caller may read. A mail
// outside the caller's content gate has no entry.
func mailOrigins(ctx context.Context, tx pgx.Tx, mails []ids.UUID) (map[ids.UUID]*crmcontracts.AttachmentOrigin, error) {
	out := make(map[ids.UUID]*crmcontracts.AttachmentOrigin, len(mails))
	if len(mails) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	set := arg(mails)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sprintf(`SELECT a.id, a.kind, a.subject, a.occurred_at FROM activity a
		 WHERE a.id = ANY($%d) AND a.archived_at IS NULL AND %s`, set, orUnbounded(content)), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading where the files came from: %w", err)
	}
	defer rows.Close()
	var readable []ids.UUID
	for rows.Next() {
		var mail ids.UUID
		origin := &crmcontracts.AttachmentOrigin{}
		if err := rows.Scan(&mail, &origin.Kind, &origin.Subject, &origin.OccurredAt); err != nil {
			return nil, fmt.Errorf("activities: reading where the files came from: %w", err)
		}
		origin.ActivityId = openapi_types.UUID(mail)
		out[mail] = origin
		readable = append(readable, mail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading where the files came from: %w", err)
	}
	if len(readable) == 0 {
		return out, nil
	}
	parties, err := readEmailPartiesOf(ctx, tx, readable)
	if err != nil {
		return nil, err
	}
	for mail, origin := range out {
		if from := parties[mail].from; len(from) > 0 {
			sender := from[0]
			origin.Sender = &sender
		}
	}
	return out, nil
}
