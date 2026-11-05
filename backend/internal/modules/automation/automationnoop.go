// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"reflect"
)

// updateChangesNothing answers whether the update leaves the automation as it
// is. Such a save must not move the version. It must not clear the reason a
// paused automation carries either: only a change of enabled state retires it.
func updateChangesNothing(before Automation, in UpdateAutomationInput) bool {
	if in.Name != nil && *in.Name != before.Name {
		return false
	}
	if in.Enabled != nil && *in.Enabled != before.Enabled {
		return false
	}
	if in.OwnerID != nil && (before.OwnerID == nil || *in.OwnerID != *before.OwnerID) {
		return false
	}
	return in.Params == nil || sameParams(before.Params, in.Params)
}

// sameParams compares the stored params with the request's as JSON values, so
// key order and number spelling do not read as a change.
func sameParams(stored json.RawMessage, sent map[string]any) bool {
	var storedValue map[string]any
	if err := json.Unmarshal(stored, &storedValue); err != nil {
		return false
	}
	raw, err := json.Marshal(sent)
	if err != nil {
		return false
	}
	var sentValue map[string]any
	if err := json.Unmarshal(raw, &sentValue); err != nil {
		return false
	}
	return reflect.DeepEqual(storedValue, sentValue)
}
