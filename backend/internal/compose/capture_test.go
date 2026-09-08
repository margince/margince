// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/testmailbox"
)

func testMailboxRegisteredWith(cfg CaptureConfig) bool {
	r := NewCaptureRegistry(nil, nil, cfg)
	return registeredNames(r.Connectors())[testmailbox.Name]
}

func TestTestMailboxRegisteredOnlyWhenAllowed(t *testing.T) {
	if testMailboxRegisteredWith(CaptureConfig{AllowTestMailbox: false}) {
		t.Error("test_mailbox is registered with AllowTestMailbox false — a production install with no such flag set must have no test_mailbox connector to reach at all")
	}
	if !testMailboxRegisteredWith(CaptureConfig{AllowTestMailbox: true}) {
		t.Error("test_mailbox is not registered with AllowTestMailbox true")
	}
}

// The zero value (an enumerate-only construction, e.g. CoreChannelProviders)
// must not accidentally arm this — false is the safe default for a
// CaptureConfig nobody deliberately built for a booted role.
func TestTestMailboxAbsentFromTheZeroValueConfig(t *testing.T) {
	if testMailboxRegisteredWith(CaptureConfig{}) {
		t.Error("test_mailbox is registered from a zero-value CaptureConfig{} — AllowTestMailbox must default to false")
	}
}
