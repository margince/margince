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
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// captureSettingsObject is the RBAC object gating the capture-settings
// surface: every role reads it (a rep needs to see whether auto-enrich is
// live), only admin/ops writes.
const captureSettingsObject = "capture_settings"

// captureAuditVerb is the verb every capture setting's audit row carries.
const captureAuditVerb = "update"

// SettingsObject is the same object, for compose.
//
// Exported because the OAuth-app transport has to take the gate BEFORE it can
// see whether the store exists — a wiring check that answers first turns the
// status code into an oracle for whether this installation has a vault. The
// unexported spelling stays the one this package uses, so there is one value
// rather than two that happen to agree.
const SettingsObject = captureSettingsObject

// AutoEnrich is the captured-company auto-enrich posture (CAP-PARAM-7,
// ADR-0072/A118).
//
// Default true preserves 0121's column default, so an installation that never
// touched the toggle sees no behaviour change across the move.
//
// One thing the move DID change: the audit row's before/after field is now
// keyed `capture.auto_enrich` where the column form wrote `auto_enrich`. The
// verb, object and actor are unchanged, but an operator reading the ledger
// across the migration boundary sees both spellings.
var AutoEnrich = config.DefineStored(config.Spec[bool]{
	Key:     "capture.auto_enrich",
	Env:     "MARGINCE_CAPTURE_AUTO_ENRICH",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Read a new contact's company web page automatically.",
}, config.Store{RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb})

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
var MailSharing = config.DefineStored(config.Spec[bool]{
	Key:     "capture.mail_sharing",
	Env:     "MARGINCE_CAPTURE_MAIL_SHARING",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let every colleague who can see a contact read the email captured with them; off keeps new mail to those on it.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, // the sink stamps each new email's audience from it
})

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
var SharedPostureAllowed = config.DefineStored(config.Spec[bool]{
	Key:     "capture.shared_posture_allowed",
	Env:     "MARGINCE_CAPTURE_SHARED_POSTURE_ALLOWED",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Allow a seat to share their mailbox with colleagues as mail lands; turn it on only where a works agreement allows it.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, // SetMailPosture reads it inside the write it gates
})

// SignatureEnrich is the workspace DEFAULT for reading contact details out of
// captured mail — a title, a phone number, an address, the company somebody
// types under their own name.
//
// It governs BOTH readers, which is why its name is narrower than its subject:
// the signature pass, and the ingest of a vCard attached to a message
// (compose/vcardingest.go reads this same key). One switch, because somebody
// turning it off means "do not take contact details off my mail", and a card
// attached to that mail is the same answer to the same question.
//
// The pass runs within minutes of a message arriving; the daily cycle is the
// backstop rather than the trigger.
//
// It is the default rather than the answer: `capture_connection.signature_enrich_enabled`
// overrides it per mailbox, and a mailbox that never chose follows this. That
// split is what a Betriebsvereinbarung negotiation asks for — a company
// can turn the whole workspace off, or leave it on and let one mailbox opt out,
// without the two answers being the same knob.
//
// Distinct from the exclusion list, which is a different lever entirely: that
// keeps whole MESSAGES out of capture by address or domain, and says nothing
// about whether captured mail may be read for contact details.
//
// Default true: the pass reads only what a contact put under their own name in
// mail they sent us, which is the least-surprising enrichment in the product.
var SignatureEnrich = config.DefineStored(config.Spec[bool]{
	Key:     "capture.signature_enrich",
	Env:     "MARGINCE_CAPTURE_SIGNATURE_ENRICH",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Read contact details from the signatures and attached cards of captured mail, for every mailbox that has made no choice of its own.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, // candidate selection reads it, per mailbox, in SQL
})

// Definitions is capture's contribution to the settings registry. Compose
// concatenates each module's list; a module that declares no settings has no
// such function, so this is opt-in rather than an interface every module must
// satisfy.
func Definitions() []settings.Definition {
	return []settings.Definition{GoogleAppSetting, MicrosoftAppSetting, UnitAppsSetting}
}

// ConfigSettings are capture's settings in the configuration registry. Named
// apart from Settings, the posture this package answers with.
func ConfigSettings() []config.Any {
	return []config.Any{
		AutoEnrich, MailSharing, SharedPostureAllowed, SignatureEnrich,
		AutoEnrichDailyCap, SiteReadMaxPages, SiteReadMaxMiB, SiteReadWallSeconds,
		MailSyncIntervalSeconds, ConfidentialityPassCap,
	}
}
