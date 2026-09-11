// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a person is told before they overrule the engine.
//
// An instruction records the WARNING VERSION somebody acknowledged, because a
// record naming no version cannot say what they were told and the text changes.
// Until now that version was a string a caller supplied and nothing published
// the words behind it — so a client could name any version it liked, and the
// record would say a person had read wording the installation never wrote.
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

// OverrideWarning is the caution a person reads before directing a refused
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
const OverrideWarningVersion = "override-v1"

// overrideWarningText is what a person is shown.
//
// It says three things, and each is there because leaving it out would let
// somebody acknowledge something untrue:
//
//   - the refusal STANDS. Directing a send is not a correction of the engine
//     and not a grant of consent; the record will still say this message was
//     refused.
//   - the DECISION IS THEIRS, by name. The instruction records who they are and
//     what they said, and it cannot be edited afterwards.
//   - it covers THIS message only. The next one to the same person is refused
//     again, so this is not a door being opened.
const overrideWarningText = "The engine refused this message, and that refusal stands: " +
	"sending it now records an exception, never consent. Your name, your reason and the exact " +
	"message are written to a record that cannot be edited afterwards. This covers this message " +
	"alone — the next one to the same recipient is judged from scratch."

// TheOverrideWarning answers the caution and its version together, because a
// surface that could get one without the other would show words while naming a
// different version.
func TheOverrideWarning() OverrideWarning {
	return OverrideWarning{Version: OverrideWarningVersion, Text: overrideWarningText}
}
