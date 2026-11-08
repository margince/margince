// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"errors"
	"fmt"
)

// ReportMessageRemovals retains failures so the caller cannot advance its cursor.
func ReportMessageRemovals(ctx context.Context, sink Sink, provider string, removed []string) error {
	remover, ok := sink.(MessageRemover)
	if !ok {
		return nil
	}
	var failures []error
	for _, id := range removed {
		if err := remover.RemoveMessage(ctx, NaturalKey{SourceSystem: provider, SourceID: id}); err != nil {
			failures = append(failures, fmt.Errorf("%s: applying a mailbox deletion: %w", provider, err))
		}
	}
	return errors.Join(failures...)
}
