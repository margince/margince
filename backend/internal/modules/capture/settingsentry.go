// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The capture module's own settings declarations (ADR-0090/A135). Capture
// owns these: it supplies the type, default, validator, RBAC object and audit
// verb, and platform/settings runs them without knowing what enrichment is.
//
// AutoEnrich moved here from a `capture_auto_enrich` column on the workspace
// row (0121). The governance is deliberately unchanged — same
// `capture_settings` object, same audit-only posture (EVT-NOEVT-3: the closed
// event catalog defines no capture-settings verb) — because this is a change
// of where the value lives, not of who may change it.

import (
	"github.com/margince/margince/backend/internal/platform/settings"
)

// captureSettingsObject is the RBAC object gating the capture-settings
// surface: every role reads it (a rep needs to see whether auto-enrich is
// live), only admin/ops writes.
const captureSettingsObject = "capture_settings"

// SettingsObject is the same object, for compose.
//
// Exported because the OAuth-app transport has to take the gate BEFORE it can
// see whether the store exists — a wiring check that answers first turns the
// status code into an oracle for whether this installation has a vault. The
// unexported spelling stays the one this package uses, so there is one value
// rather than two that happen to agree.
const SettingsObject = captureSettingsObject

// AutoEnrich is the captured-organization auto-enrich posture (CAP-PARAM-7,
// ADR-0072/A118).
//
// Default true preserves 0121's column default, so an installation that never
// touched the toggle sees no behaviour change across the move.
//
// One thing the move DID change: the audit row's before/after field is now
// keyed `capture.auto_enrich` where the column form wrote `auto_enrich`. The
// verb, object and actor are unchanged, but an operator reading the ledger
// across the migration boundary sees both spellings.
var AutoEnrich = settings.Define[bool](
	"capture.auto_enrich",
	captureSettingsObject,
	"update",
	true,
	nil, // every bool is a valid posture; a validator here would be ceremony
)

// MailSharing is the workspace mail-sharing posture: ON, a captured email is
// readable by every colleague who can see the contact; OFF, every email
// captured FROM THEN ON is held to its participants and the capturing mailbox
// owner (the sink stamps audience='participants' at insert). Already-captured
// mail keeps the audience it has — the setting moves the default, it rewrites
// no history.
//
// Default true: sharing is the point of a CRM, and the off position exists
// for installations that accept how much harder it makes shared pipeline
// work — the settings surface says so in as many words.
var MailSharing = settings.Define[bool](
	"capture.mail_sharing",
	captureSettingsObject,
	"update",
	true,
	nil, // every bool is a valid posture; a validator here would be ceremony
).MachineryApplied() // the sink stamps each new email's audience from it

// SharedPostureAllowed is the workspace's answer to whether a seat may put
// their mailbox in the `shared` posture at all — colleagues reading a captured
// message the moment it lands, before anything has judged it.
//
// Default FALSE, and it is the only capture setting whose default withholds
// rather than shares. Reading an employee's mailbox into a shared CRM is what
// the DACH works-council agreement is about, and a product that switched it on
// by itself would be making a legal commitment on the customer's behalf. The
// admin confirming it is the customer saying they hold that agreement; the
// product does not verify one exists, and says so where it asks.
var SharedPostureAllowed = settings.Define[bool](
	"capture.shared_posture_allowed",
	captureSettingsObject,
	"update",
	false,
	nil, // every bool is a valid posture; a validator here would be ceremony
).MachineryApplied() // SetMailPosture reads it inside the write it gates

// SignatureEnrich is the workspace DEFAULT for the nightly pass that lifts
// stated fields out of an email signature — a title, a phone number, the
// company somebody types under their own name.
//
// It is the default rather than the answer: `capture_connection.signature_enrich_enabled`
// overrides it per mailbox, and a mailbox that never chose follows this. That
// split is what a Betriebsvereinbarung negotiation asks for — an organization
// can turn the whole workspace off, or leave it on and let one mailbox opt out,
// without the two answers being the same knob.
//
// Distinct from the exclusion list, which is a different lever entirely: that
// keeps whole MESSAGES out of capture by address or domain, and says nothing
// about whether captured mail may be read for a signature.
//
// Default true: the pass reads only what a person put under their own name in
// mail they sent us, which is the least-surprising enrichment in the product.
var SignatureEnrich = settings.Define[bool](
	"capture.signature_enrich",
	captureSettingsObject,
	"update",
	true,
	nil, // every bool is a valid posture, as for MailSharing above
).MachineryApplied() // candidate selection reads it, per mailbox, in SQL

// Definitions is capture's contribution to the settings registry. Compose
// concatenates each module's list; a module that declares no settings has no
// such function, so this is opt-in rather than an interface every module must
// satisfy.
func Definitions() []settings.Definition {
	return []settings.Definition{
		AutoEnrich, MailSharing, SharedPostureAllowed, SignatureEnrich,
		GoogleAppSetting, MicrosoftAppSetting,
	}
}
