// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a human is told before they overrule the engine.
//
// An instruction records the WARNING VERSION somebody acknowledged, because a
// record naming no version cannot say what they were told and the text changes.
// Until now that version was a string a caller supplied and nothing published
// the words behind it — so a client could name any version it liked, and the
// record would say a human had read wording the installation never wrote.
//
// The warning is served from here instead. A surface shows what it is given and
// echoes the version back; the record then names text that exists, and a dispute
// about an override can be answered with the words themselves.
//
// NOT A TEMPLATE, and not in consent_text_version. Those are the wording
// published to DATA SUBJECTS — the sentence in a confirmation mail, the
// disclosure on a preference page — and they are per-locale rows an erasure and
// a proof both reach. This is an internal caution shown to an employee about a
// decision they are making, which belongs in the build rather than in a table
// somebody's installation can edit.
//
// THE VERSION MOVES WHEN THE WORDS DO. That is the whole contract: an
// instruction saying "override-v1" must mean one fixed text forever, so
// changing the sentence without moving the version would silently rewrite what
// every past acknowledgement claims to have said. A gate holds it.

// OverrideWarning is the caution a human reads before directing a refused
// send, and the version their acknowledgement will name.
type OverrideWarning struct {
	Version string
	Text    string
}

// OverrideWarningVersion is the version this build serves.
//
// MOVE IT WHENEVER THE TEXT BELOW CHANGES, in the same commit. An instruction
// naming a version whose words have since been edited is a record of an
// acknowledgement nobody made.
const OverrideWarningVersion = "override-v2"

// overrideWarningText is what a human is shown, beside the specific reason
// the check gave.
//
// It says three things, and each is there because leaving it out would let
// somebody acknowledge something untrue:
//
//   - the refusal STANDS. An exception is not a correction of the check and
//     not a grant of consent.
//   - it covers THIS message only, and no later one.
//   - the record is KEPT: their name, their explanation and the message, in a
//     record nobody can edit.
const overrideWarningText = "The send check still blocks this message. " +
	"This sends only this message. Your name, your explanation and the message are kept " +
	"in a record that cannot be edited. It does not grant consent or permission for future messages."

// overrideWarningTextV1 is the text override-v1 named. Kept word for word,
// because an instruction recorded under it says somebody read these words.
const overrideWarningTextV1 = "The engine refused this message, and that refusal stands: " +
	"sending it now records an exception, never consent. Your name, your reason and the exact " +
	"message are written to a record that cannot be edited afterwards. This covers this message " +
	"alone — the next one to the same recipient is judged from scratch."

// overrideWarnings resolves the words a recorded version showed. A
// version is added and never edited or removed. A record naming any of them
// can then be read back with what its director was shown. Only the current one is
// accepted for a new instruction (servedWarningVersion).
//
// Held by: TestEveryServedWarningVersionReadsBackAndOnlyTheCurrentOneIsAccepted (backend/internal/modules/consent/overridewarning_test.go)
var overrideWarnings = map[string]string{
	"override-v1":          overrideWarningTextV1,
	OverrideWarningVersion: overrideWarningText,
}

// OverrideWarningFor answers the words a version named, and false for a
// version this build never served.
func OverrideWarningFor(version string) (OverrideWarning, bool) {
	text, known := overrideWarnings[version]
	return OverrideWarning{Version: version, Text: text}, known
}

// TheOverrideWarning answers the caution and its version together, because a
// surface that could get one without the other would show words while naming a
// different version.
func TheOverrideWarning() OverrideWarning {
	return OverrideWarning{Version: OverrideWarningVersion, Text: overrideWarnings[OverrideWarningVersion]}
}
