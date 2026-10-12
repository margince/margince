// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"testing"
)

func TestMeAdvertisesReporting(t *testing.T) {
	response := NewHandlers(&Service{}).meResponse(context.Background(), Identity{SeatType: "full"})
	if response.SettingsAvailability.Reporting == nil || !*response.SettingsAvailability.Reporting {
		t.Fatal("older clients must see reporting available without deployment options")
	}
}
