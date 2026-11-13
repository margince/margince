// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The zone is stored as sent, so a spelling the parser would trim is a field
// error here rather than a database check failure later.
func TestAPaddedZoneNameIsRefusedAsAFieldError(t *testing.T) {
	for _, zone := range []string{" Europe/Berlin", "Europe/Berlin\n"} {
		_, err := ResolveIntentTime("due_intent", crmcontracts.IntentTime{WallClock: "2026-10-05T09:00", Timezone: zone})
		parseErr, ok := errors.AsType[*values.ParseError](err)
		if !ok || parseErr.Field != "due_intent.timezone" {
			t.Errorf("zone %q: err = %v", zone, err)
		}
	}
	if _, err := ResolveIntentTime("due_intent", crmcontracts.IntentTime{WallClock: "2026-10-05T09:00", Timezone: "Europe/Berlin"}); err != nil {
		t.Fatalf("canonical zone refused: %v", err)
	}
}
