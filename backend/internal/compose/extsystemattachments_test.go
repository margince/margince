// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// attachmentReads calls both attachment reads with the same seat and id.
func attachmentReads(ctx context.Context, rt extension.Runtime, actAs extension.UserID, id string) map[string]error {
	_, list := rt.Activities().Attachments(ctx, actAs, id)
	_, open := rt.Activities().OpenAttachment(ctx, actAs, id)
	return map[string]error{"Attachments": list, "OpenAttachment": open}
}

// A unit reads attachments only when it declares the group, only where no user
// gets the answer, and only as a seat it names.
func TestTheAttachmentReadsNeedTheirDeclarationAnUnattendedCallAndASeat(t *testing.T) {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	binding := extensionRuntimeBinding{pool: &pgxpool.Pool{}}
	seat, id := extension.UserID(ids.NewV7().String()), ids.NewV7().String()
	expect := func(what string, got map[string]error, want error) {
		t.Helper()
		for read, err := range got {
			if !errors.Is(err, want) {
				t.Errorf("%s %s: err = %v, want %v", what, read, err, want)
			}
		}
	}

	composeReader(t, extension.SystemReadMailContent)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	expect("an undeclared", attachmentReads(ctx, undeclared, seat, id), extension.ErrUndeclaredRead)

	composeReader(t, extension.SystemReadMailAttachments)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	expect("a signed-in", attachmentReads(ctx, attended, seat, id), extension.ErrAttendedRead)

	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	expect("a seatless", attachmentReads(ctx, declared, "", id), extension.ErrInvalid)
	expect("a malformed seat's", attachmentReads(ctx, declared, "not-a-seat", id), extension.ErrInvalid)
}
