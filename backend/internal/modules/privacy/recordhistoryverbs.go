// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The audit vocabulary as a reader sees it: one past-tense phrase per verb the
// audit_log CHECK admits.
//
// Its own file because it is its own concept — a translation table with a gate
// over it — and because it is the half of record history that GROWS. Every new
// audit verb lands here and nowhere else, and it was the growth that pushed
// recordhistory.go past the file-length cap.

// recordHistoryVerbs renders each audit_log action as a past-tense phrase
// for composeRecordSummary. The set here is reconciled to the CHECK
// constraint's admitted vocabulary;
// TestRecordHistoryVerbsCoverTheAuditCheckVocabulary derives that
// vocabulary from the highest-numbered migration restating
// audit_log_action_check and fails if a future widening lands without a
// matching entry here. An action absent from the map (defensive only — the
// CHECK already closes the set at the DB level) falls back to the raw
// string, never an error: an unrenderable phrase is still honest history.
var recordHistoryVerbs = map[string]string{ // #nosec G101 -- audit verbs and the English phrases they render as; "password_link_issued" names an action, and no value here is a secret
	"create":      "created",
	"update":      "updated",
	actionArchive: "archived",
	"merge":       "merged",
	"promote":     "promoted",
	"restore":     "restored",
	"export":      "exported",
	"erase":       "erased",
	// A hard delete, distinct from both neighbours above it. "archived" would
	// say the row is still there, and "erased" is this product's Art. 17 word —
	// a reader scanning a trail for subject erasures must not find a corpus
	// document's removal among them.
	"delete":        "deleted",
	"login":         "logged in",
	"assign":        "assigned",
	"advance_stage": "advanced the stage of",
	"advance_phase": "advanced the phase of",
	// Deal Room access. "invited" and "revoked access for" rather than
	// "created"/"archived": the row is incidental, the access is the fact.
	"invite": "invited",
	"revoke": "revoked access for",
	// The Deal Room lifecycle. "closed" rather than "ended", because closing a
	// room keeps the buyer reading it — the phrase should not suggest access
	// went away with the content freeze.
	"publish":          "published",
	"pause":            "paused",
	"resume":           "resumed",
	"close":            "closed",
	"approve":          "approved",
	"accrue":           "accrued commission on",
	"pay":              "paid",
	"reject":           "rejected",
	"consent_grant":    "granted consent for",
	"consent_withdraw": "withdrew consent for",
	"activity_relink":  "relinked",
	"record_share":     "shared",
	"record_unshare":   "unshared",
	"resolve":          "resolved",
	"demote":           "demoted",
	"import":           "imported",
	"import_undo":      "undid the import of",
	"disqualify":       "disqualified",
	"anonymize":        "anonymized",
	"send_email":       "sent an email for",
	"reset_data":       "reset",
	// Reads as "<admin> issued a set-password link for <member>" — the phrase
	// names what was issued, because on a member's history "issued" alone
	// would not distinguish a credential from anything else granted to them.
	"password_link_issued": "issued a set-password link for",
	// The subject is the provider connection, so the phrases name what the act
	// did to the installation's relationship with that provider rather than to
	// a record: connecting binds a paid credential, disconnecting destroys it
	// and stops all egress.
	"connect":    "connected",
	"disconnect": "disconnected",
	// A message the sender chose to send later, through its four acts
	// (ADR-0104/A155). "released" rather than "sent": the provider has not been
	// called when this is written.
	"schedule":   "scheduled",
	"reschedule": "rescheduled",
	"cancel":     "cancelled",
	"release":    "released for sending",
	"hold":       "held for review",
	// Nobody did this, and the phrase has to say so while still composing into
	// "<actor> <verb> the record" — "expired unactioned the record" is not a
	// sentence. Not "rejected", because the ledger's job here is to tell a
	// reader that no one ANSWERED: a column of these is work going unattended,
	// which no count of rejections would show.
	"expire": "let the decision window close on",
	// The statutory-retention pair (A165/ADR-0114, migration 0287). The
	// formatter is "<actor> <verb> the record", so a phrase has to take "the
	// record" as its direct object and read as a sentence: "System withheld the
	// record" and "Lars pinned the record" both do. The longer phrasing this
	// pair wants — naming the statutory obligation — cannot go here, because it
	// would strand the object ("withheld under a statutory retention obligation
	// the record"). It belongs in the evidence the restricted-records list
	// renders, which has room for the basis and the deadline.
	//
	// "withheld" rather than "restricted", which a reader hears as an
	// access-control decision the business made; this is the opposite — an
	// obligation the business is under, holding a record it would otherwise
	// have had to erase.
	//
	// A165's other two verbs, `release` and `expire`, are NOT re-keyed here and
	// cannot be: this map keys on the verb alone, and both already carry
	// unrelated phrases from the scheduled-send feature ("released for sending",
	// written on scheduled_send by activities/scheduledsendfire.go) and from
	// approvals ("let the decision window close on"). A retention release would
	// render as a sending event. Resolving that needs the phrase to key on
	// (verb, entity_type) rather than verb, which is a change to this map's
	// shape and to composeRecordSummary — it belongs with the code that first
	// writes a retention release, not ahead of it.
	"restrict": "withheld",
	"pin":      "pinned for statutory retention",
}
