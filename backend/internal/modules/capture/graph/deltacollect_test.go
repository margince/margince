// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"time"
)

// collectAnchor walks a fresh inbox anchor round and gathers its pages, for tests
// that read what a whole round named.
func collectAnchor(ctx context.Context, api API, token string, after time.Time) ([]MessageRef, string, error) {
	var messages []MessageRef
	link, err := api.DeltaInit(ctx, token, folderInbox, after, nil, func(_ context.Context, page DeltaPage) error {
		messages = append(messages, page.Messages...)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return messages, link, nil
}

// collectDelta walks an incremental round and gathers its pages.
func collectDelta(ctx context.Context, api API, token, deltaLink string) ([]MessageRef, string, error) {
	var messages []MessageRef
	link, err := api.Delta(ctx, token, deltaLink, nil, func(_ context.Context, page DeltaPage) error {
		messages = append(messages, page.Messages...)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return messages, link, nil
}
