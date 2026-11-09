// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A mail logged by hand as a reply to another mail is filed on that mail's
// thread. The thread answer rule (storekit.AnsweredSQL) then reads it as the
// answer. Nothing else about the answer rule changes: an off-thread reply still
// needs attested sending.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownThreadPrefix names the threads a hand-logged mail starts. Provider thread
// keys are RFC 5322 Message-IDs, which always hold an "@", and channel keys
// name their provider first, so no captured key can take this form.
const ownThreadPrefix = "logged:"

// ownThreadKey is the thread a hand-logged mail with no identity starts.
func ownThreadKey(id ids.UUID) string { return ownThreadPrefix + id.String() }

// ReplyFieldError refuses in_reply_to_activity_id, naming why.
type ReplyFieldError struct{ Field, Message string }

func (e *ReplyFieldError) Error() string { return e.Message }

// FieldFault names the refused reply target for HTTP and tool callers.
func (e *ReplyFieldError) FieldFault() (field, code, message string) {
	return e.Field, faultInvalid, e.Message
}

// threadOnReplyTx files the reply on the thread of the mail it answers. The
// mail must be one the caller can read, an email, and with the same
// correspondent. A mail logged before hand-logged mail kept a thread has none.
// The reply then starts its own, and is never refused over old data.
func threadOnReplyTx(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if in.InReplyTo == nil {
		return nil
	}
	refuse := func(message string) error { return &ReplyFieldError{Field: fieldInReplyTo, Message: message} }
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, *in.InReplyTo); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return refuse("Choose a mail you can read.")
		}
		return err
	}
	var kind, threadKey, counterparty string
	if err := tx.QueryRow(ctx, `
		SELECT kind, coalesce(thread_key, ''), coalesce(counterparty_email, '')
		  FROM activity WHERE id = $1`, *in.InReplyTo).Scan(&kind, &threadKey, &counterparty); err != nil {
		return fmt.Errorf("activities: reading the mail a reply answers: %w", err)
	}
	if kind != string(crmcontracts.ActivityKindEmail) {
		return refuse("Choose an email to reply to.")
	}
	same, err := sameCorrespondentTx(ctx, tx, in, *in.InReplyTo, counterparty)
	if err != nil {
		return err
	}
	if !same {
		return refuse("Choose a mail from the same correspondent as this reply.")
	}
	in.ThreadKey = threadKey
	return nil
}

// sameCorrespondentTx answers whether the reply is with the mail's
// correspondent. That is the same counterparty address when both state one,
// or else a contact both are filed under.
func sameCorrespondentTx(ctx context.Context, tx pgx.Tx, in *LogActivityInput, original ids.UUID, theirs string) (bool, error) {
	ours, err := counterpartyFor(ctx, tx, *in)
	if err != nil {
		return false, err
	}
	if ours != "" && theirs != "" {
		return ours == theirs, nil
	}
	var contacts []ids.UUID
	for _, link := range in.Links {
		if link.EntityType == linkEntityContact {
			contacts = append(contacts, link.EntityID)
		}
	}
	var shared bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity_link
		                WHERE activity_id = $1 AND contact_id = ANY($2))`, original, contacts).Scan(&shared)
	if err != nil {
		return false, fmt.Errorf("activities: reading who the replied mail was with: %w", err)
	}
	return shared, nil
}
