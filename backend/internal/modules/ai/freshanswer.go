// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "context"

type freshAnswerKey struct{}

// WithFreshAnswer marks the call ctx carries so the model answers it again,
// even when the result cache holds a valid answer to the same request. A caller
// that asks twice because the first answer was not enough uses it. The cache
// would hand the first answer back, and the second question would be no
// question. The new answer is cached in its place.
func WithFreshAnswer(ctx context.Context) context.Context {
	return context.WithValue(ctx, freshAnswerKey{}, true)
}

// FreshAnswerAsked reports whether the call ctx carries asked for a fresh answer.
func FreshAnswerAsked(ctx context.Context) bool {
	asked, _ := ctx.Value(freshAnswerKey{}).(bool)
	return asked
}
