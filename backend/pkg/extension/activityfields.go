// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//margince:extension-surface

package extension

import "time"

// ActivityFields preserves the provider's timeline content and privacy hold.
type ActivityFields struct {
	ProviderPrivate bool
	Kind            string
	ChannelProvider string
	Subject         string
	Body            string
	OccurredAt      time.Time
	DateOnly        bool
	DurationSeconds *int
	Direction       string
}
