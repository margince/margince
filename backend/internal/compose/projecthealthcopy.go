// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The words the project health job writes: the one-sentence reason a reading
// carries, and the owner's notice. Both are stored once and read by everyone
// who can see the project, so they follow the installation's base language,
// in the shape signalsummarycopy.go keeps.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// projectHealthCopy is one language's set. Every field is required.
type projectHealthCopy struct {
	// quiet, overdue, quietAndOverdue and active are the reasons. %d is the
	// quiet rule's window in days.
	quiet           string
	overdue         string
	quietAndOverdue string
	active          string
	// noticeSubject heads the owner's notice. %s is the project's name, then
	// the state's word, in that order.
	noticeSubject string
	// noticeUndo follows the reason in the notice body.
	noticeUndo string
	// The state words, as the notice subject says them.
	onTrack, atRisk, offTrack string
}

var projectHealthByLang = map[textlang.Lang]projectHealthCopy{
	textlang.English: {
		quiet:           systemtext.Copy("project.quiet", textlang.English),
		overdue:         systemtext.Copy("project.overdue", textlang.English),
		quietAndOverdue: systemtext.Copy("project.quietAndOverdue", textlang.English),
		active:          systemtext.Copy("project.active", textlang.English),
		noticeSubject:   systemtext.Copy("project.noticeSubject", textlang.English),
		noticeUndo:      systemtext.Copy("project.noticeUndo", textlang.English),
		onTrack:         systemtext.Copy("project.onTrack", textlang.English), atRisk: systemtext.Copy("project.atRisk", textlang.English), offTrack: systemtext.Copy("project.offTrack", textlang.English),
	},
	textlang.German: {
		quiet:           systemtext.Copy("project.quiet", textlang.German),
		overdue:         systemtext.Copy("project.overdue", textlang.German),
		quietAndOverdue: systemtext.Copy("project.quietAndOverdue", textlang.German),
		active:          systemtext.Copy("project.active", textlang.German),
		noticeSubject:   systemtext.Copy("project.noticeSubject", textlang.German),
		noticeUndo:      systemtext.Copy("project.noticeUndo", textlang.German),
		onTrack:         systemtext.Copy("project.onTrack", textlang.German), atRisk: systemtext.Copy("project.atRisk", textlang.German), offTrack: systemtext.Copy("project.offTrack", textlang.German),
	},
	textlang.Vietnamese: {
		quiet:           systemtext.Copy("project.quiet", textlang.Vietnamese),
		overdue:         systemtext.Copy("project.overdue", textlang.Vietnamese),
		quietAndOverdue: systemtext.Copy("project.quietAndOverdue", textlang.Vietnamese),
		active:          systemtext.Copy("project.active", textlang.Vietnamese),
		noticeSubject:   systemtext.Copy("project.noticeSubject", textlang.Vietnamese),
		noticeUndo:      systemtext.Copy("project.noticeUndo", textlang.Vietnamese),
		onTrack:         systemtext.Copy("project.onTrack", textlang.Vietnamese), atRisk: systemtext.Copy("project.atRisk", textlang.Vietnamese), offTrack: systemtext.Copy("project.offTrack", textlang.Vietnamese),
	},
}

// projectHealthCopyFor answers the set for lang, falling back to English for a
// language the product does not ship.
func projectHealthCopyFor(lang textlang.Lang) projectHealthCopy {
	if said, ok := projectHealthByLang[lang]; ok {
		return said
	}
	return projectHealthByLang[textlang.English]
}

func (words projectHealthCopy) state(state crmcontracts.ProjectHealthState) string {
	switch state {
	case crmcontracts.ProjectHealthStateOffTrack:
		return words.offTrack
	case crmcontracts.ProjectHealthStateAtRisk:
		return words.atRisk
	default:
		return words.onTrack
	}
}
