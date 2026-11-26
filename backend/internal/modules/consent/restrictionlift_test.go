// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALiftNoticeNamesTheStopItReleases(t *testing.T) {
	stop := ids.NewV7()
	got, ok := restrictionLiftStop(restrictionLiftMessageID(stop, "20261112-42"))
	if !ok || got != stop {
		t.Fatalf("read back %v (%v), want %v", got, ok, stop)
	}
	for _, other := range []string{
		acknowledgementMessageID(stop),
		"restriction-lift-nothex@margince.invalid",
		"",
	} {
		if _, ok := restrictionLiftStop(other); ok {
			t.Errorf("%q read as a lift notice", other)
		}
	}
}
