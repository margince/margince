// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestBackfillStalledKeepsEveryMessageAtTheStrikeLimit(t *testing.T) {
	got := backfillStalled([]byte(`{"page_token":"p","stalls":{"b":3,"a":4,"c":2}}`))
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("got %q", got)
	}
	if got := backfillStalled(nil); got != nil {
		t.Fatalf("no cursor must read as no stall, got %q", got)
	}
}

func TestFaultedMessageSeesThroughWrapping(t *testing.T) {
	err := fmt.Errorf("page: %w", &connector.MessageFaultError{ID: "m9", Err: connector.ErrUnreachable})
	if got := faultedMessage(err); got != "m9" {
		t.Fatalf("got %q", got)
	}
	if got := faultedMessage(connector.ErrUnreachable); got != "" {
		t.Fatalf("got %q", got)
	}
}
