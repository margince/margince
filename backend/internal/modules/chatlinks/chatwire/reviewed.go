// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import "context"

// ReviewedWriter requires the expected audience alongside shared provider writes.
type ReviewedWriter interface {
	PostReviewed(context.Context, string, string, string, string, string) (Posted, error)
	UpdateReviewed(context.Context, string, string, string, string) error
}
