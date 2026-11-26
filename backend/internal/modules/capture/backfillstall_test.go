// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestBackfillStallReadsTheMessageAndItsStrikes(t *testing.T) {
	if id, n := backfillStall([]byte(`{"page_token":"p","stall_id":"m1","stall_count":3}`)); id != "m1" || n != backfillMessageStrikes {
		t.Fatalf("got %q %d", id, n)
	}
	if id, n := backfillStall(nil); id != "" || n != 0 {
		t.Fatalf("no cursor must read as no stall, got %q %d", id, n)
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
