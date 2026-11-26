// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The worklist notices the product raises for itself.
//
// Its own file rather than another section of catalog.go, which is at the
// length a reader can hold. The catalog's sections are one function per
// message for the same reason: a translator opens the section they are
// working on.

// noticeLines is what a worklist notice the product raises says.
func noticeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.NoticeSLASubject },
		systemtext.Copy("notice.NoticeSLASubject", textlang.English),
		systemtext.Copy("notice.NoticeSLASubject", textlang.German),
		systemtext.Copy("notice.NoticeSLASubject", textlang.Vietnamese))
	line(func(c *Copy) *string { return &c.NoticeSLABody },
		systemtext.Copy("notice.NoticeSLABody", textlang.English),
		systemtext.Copy("notice.NoticeSLABody", textlang.German),
		systemtext.Copy("notice.NoticeSLABody", textlang.Vietnamese))
	line(func(c *Copy) *string { return &c.NoticeBacklogSubject },
		systemtext.Copy("notice.NoticeBacklogSubject", textlang.English),
		systemtext.Copy("notice.NoticeBacklogSubject", textlang.German),
		systemtext.Copy("notice.NoticeBacklogSubject", textlang.Vietnamese))
	// The count leads as a label rather than a quantity in a sentence.
	//
	// One waiting decision is reachable, because the seats query counts and
	// groups with no lower bound. A sentence would read "1 sender decisions".
	// Vietnamese does not inflect for number and reads naturally either way.
	line(func(c *Copy) *string { return &c.NoticeBacklogBody },
		systemtext.Copy("notice.NoticeBacklogBody", textlang.English),
		systemtext.Copy("notice.NoticeBacklogBody", textlang.German),
		systemtext.Copy("notice.NoticeBacklogBody", textlang.Vietnamese))
}
