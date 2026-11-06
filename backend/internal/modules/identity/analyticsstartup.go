// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type AnalyticsStartup func(context.Context) (*crmcontracts.AnalyticsStartup, error)

func (h Handlers) WithAnalyticsStartup(read AnalyticsStartup) Handlers {
	h.analyticsStartup = read
	return h
}

func (h Handlers) readAnalyticsStartup(ctx context.Context) *crmcontracts.AnalyticsStartup {
	out, err := h.analyticsStartup(ctx)
	if err != nil {
		// Optional preparation must not turn a valid session into an auth failure.
		// The individual screen reads still report their errors and offer retry.
		slog.WarnContext(ctx, "Analytics startup data could not be prepared", "error", err)
		return nil
	}
	return out
}
