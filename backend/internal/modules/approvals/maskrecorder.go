// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"sync/atomic"
)

type maskRecorderKey struct{}

// withMaskRecorder lets the preview builders say they masked a value, which
// the staged call would repeat. The value's kind cannot say it: an archived
// seat shows the same kind without anything being masked.
func withMaskRecorder(ctx context.Context) (context.Context, *atomic.Bool) {
	masked := &atomic.Bool{}
	return context.WithValue(ctx, maskRecorderKey{}, masked), masked
}

// RecordMasked notes that the preview being built replaced a value the
// reader's role masks.
func RecordMasked(ctx context.Context) {
	if masked, ok := ctx.Value(maskRecorderKey{}).(*atomic.Bool); ok {
		masked.Store(true)
	}
}
