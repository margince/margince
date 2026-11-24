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

// A unit reads attachments only when declared, unattended and as a seat it names.
func TestTheAttachmentReadsNeedTheirDeclarationAnUnattendedCallAndASeat(t *testing.T) {
	ctx, binding := principal.WithWorkspaceID(context.Background(), ids.NewV7()), extensionRuntimeBinding{pool: &pgxpool.Pool{}}
	seat, id := extension.UserID(ids.NewV7().String()), ids.NewV7().String()
	expect := func(what string, rt extension.Runtime, actAs extension.UserID, want error) {
		t.Helper()
		_, list := rt.Activities().Attachments(ctx, actAs, id)
		_, open := rt.Activities().OpenAttachment(ctx, actAs, id)
		if !errors.Is(list, want) || !errors.Is(open, want) {
			t.Errorf("%s: Attachments %v, OpenAttachment %v; want %v", what, list, open, want)
		}
	}
	composeReader(t, extension.SystemReadMailContent)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	expect("an undeclared read", undeclared, seat, extension.ErrUndeclaredRead)
	composeReader(t, extension.SystemReadMailAttachments)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	expect("a signed-in call", attended, seat, extension.ErrAttendedRead)
	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	expect("no seat", declared, "", extension.ErrInvalid)
	expect("a malformed seat", declared, "not-a-seat", extension.ErrInvalid)
}
