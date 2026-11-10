// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/platform/corefeatures"

// WithCoreFeatures publishes availability separately from stored role grants.
func (h Handlers) WithCoreFeatures(availability corefeatures.Availability) Handlers {
	h.coreFeatures = availability
	return h
}
