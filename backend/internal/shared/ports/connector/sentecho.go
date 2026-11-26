// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "context"

// SentEcho carries the provider-filed message currently being captured, not a claimed Message-ID alone.
type SentEcho struct {
	Identity string
	Subject  string
	Body     string
}

type sentEchoKey struct{}

// WithSentEcho lets receipt/capture repair require the provider-filed copy.
func WithSentEcho(ctx context.Context, echo SentEcho) context.Context {
	return context.WithValue(ctx, sentEchoKey{}, echo)
}

// SentEchoFrom preserves the distinction between a filed copy and an identity claim alone.
func SentEchoFrom(ctx context.Context) (SentEcho, bool) {
	echo, ok := ctx.Value(sentEchoKey{}).(SentEcho)
	return echo, ok
}
