// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import "fmt"

// MarketingMode is how a jurisdiction treats advertising to a recipient who has
// said nothing. Closed: Validate refuses any other value.
type MarketingMode string

const (
	// MarketingOptIn needs the recipient's prior consent, or an exception the
	// pack grants. It is the meaning of the empty value.
	MarketingOptIn MarketingMode = "opt_in"
	// MarketingOptOut allows advertising until the recipient refuses. A refusal
	// already on record still stops it.
	MarketingOptOut MarketingMode = "opt_out"
)

// Validate refuses a mode the engine does not know. The empty value is valid.
func (m MarketingMode) Validate() error {
	switch m {
	case "", MarketingOptIn, MarketingOptOut:
		return nil
	}
	return fmt.Errorf("marketing mode %q is unknown: write %q or %q", string(m), string(MarketingOptIn), string(MarketingOptOut))
}

// OptOut reports whether advertising is allowed until the recipient refuses.
func (m MarketingMode) OptOut() bool { return m == MarketingOptOut }
