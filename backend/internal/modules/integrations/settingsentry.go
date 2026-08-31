// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// The integrations module's own settings declarations (ADR-0090/A135).
//
// One setting lives here, and it carries a legal posture rather than a
// convenience: whether this installation buys the free half of a data
// provider's answer about a contact without anybody asking for it.

import (
	"github.com/margince/margince/backend/internal/platform/settings"
)

// AutomaticLookup decides whether a provider run may be queued without a human
// pressing anything — on a contact's creation, and by the catch-up sweep.
//
// Default true: the product's value is that a contact arrives already knowing
// what a provider can say for free about them, and an installation that never
// opens this screen should get that.
//
// OFF is not a pause button, it is a jurisdiction answer. Vietnamese law
// forbids trading personal data, and buying broker data about a Vietnamese
// contact is the sharpest form of that; an installation whose contacts fall
// under it turns this off and looks a contact up by hand instead, which keeps
// the decision with the person who made it. There is no per-contact fence
// because there is nothing to fence on: a person's country is not a fact this
// product holds, and deriving one from an email domain would be wrong in both
// directions while looking like protection.
//
// It governs the AUTOMATIC path only. The button on a contact's Data & tools
// tab is a human's own act and never consults this.
//
// MachineryApplied because admission reads it to bind its OWN write, inside
// the transaction that queues the run — the shape that declaration exists for.
// The posture is a fact about the installation, so it must bind the same way
// whoever the acting principal turns out to be; a read gated on the caller
// would make an installation-wide answer depend on who typed the contact.
//
// Note what this is NOT: a way around a refusal. The automatic path already
// runs under a system principal, which auth.Require admits unconditionally, so
// a gated read there would pass anyway. The declaration is about the question
// being the machinery's rather than the caller's. The settings surface answers
// this value to a caller through Get, and that read keeps its gate.
var AutomaticLookup = settings.Define[bool](
	"integrations.automatic_lookup",
	objectIntegrations,
	"update",
	true,
	nil, // every bool is a valid posture; a validator here would be ceremony
).MachineryApplied()

// Definitions is this module's contribution to the settings catalog.
func Definitions() []settings.Definition {
	return []settings.Definition{AutomaticLookup}
}
