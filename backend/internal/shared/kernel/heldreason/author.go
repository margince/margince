// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package heldreason

import (
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefuseAnotherSeat lets only a held message's author send it anyway. A
// release sends from the caller's mailbox, with their name and signature. So a
// colleague or admin may not release it, whatever their grant. activities asks
// it when the held row is claimed, and consent before a decision is recorded or
// offered.
func RefuseAnotherSeat(caller, author ids.UUID) error {
	if author.IsZero() || caller != author {
		return &AuthorOnlyError{}
	}
	return nil
}

// AuthorOnlyError refuses a caller who is not the held message's author. It
// answers 403 with a sentence naming who may send it.
type AuthorOnlyError struct{}

func (*AuthorOnlyError) Error() string {
	return "held send: only the author of a held message can send it"
}

// Is keeps the refusal on the permission sentinel's 403 row.
func (*AuthorOnlyError) Is(target error) bool { return target == apperrors.ErrPermissionDenied }

// MessageFault says who may send it.
func (*AuthorOnlyError) MessageFault() (code, message string) {
	return "permission_denied", "Only the author of this message can send it."
}
