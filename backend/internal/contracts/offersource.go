// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package crmcontracts

import "encoding/json"

// OfferSourceOnly reports a correction that changed no offer terms or content.
// Consumers leave chat and commercial readings alone for this metadata repair.
func OfferSourceOnly(raw json.RawMessage) bool {
	var update PublicEventOfferUpdated
	return json.Unmarshal(raw, &update) == nil && update.Source != nil && len(update.ChangedFields) == 1 && update.ChangedFields[0] == "source"
}
