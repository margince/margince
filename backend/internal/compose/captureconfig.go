// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/jackc/pgx/v5/pgxpool"

// WithCaptureConfig applies the same capture rules to every registry.
// Apply it before options that build a registry; omission keeps the pinned defaults.
func WithCaptureConfig(cfg CaptureConfig) Option {
	return func(s *Server, _ *pgxpool.Pool) { s.captureConfig = cfg }
}
