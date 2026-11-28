// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func chatJobContext(ctx context.Context) context.Context {
	if _, ok := principal.CorrelationID(ctx); !ok {
		ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	}
	return principal.WithActor(ctx, chatLinksPrincipal)
}
