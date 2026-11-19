// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"time"
)

// AnswerGrace is how long before its write deadline a handler's context ends.
// A handler stopped AT the write deadline can no longer write anything. Its
// caller then gets a closed socket instead of an error it can act on.
const AnswerGrace = 2 * time.Second

// WithinResponseDeadline bounds a handler's context for a response whose write
// deadline is `within` from now. The context ends AnswerGrace earlier, so the
// handler still has time to answer that it ran out of time.
func WithinResponseDeadline(ctx context.Context, within time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, within-AnswerGrace)
}
