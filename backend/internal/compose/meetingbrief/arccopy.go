// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var arcWords = struct{ One, Many, OneTitle, ManyTitle langcopy.Phrase }{
	One:       langcopy.Phrase{En: "%s: 1 conversation.", De: "%s: ein Gespräch.", Vi: "%s: một cuộc trao đổi."},
	Many:      langcopy.Phrase{En: "%s: %d conversations.", De: "%s: %d Gespräche.", Vi: "%s: %d cuộc trao đổi."},
	OneTitle:  langcopy.Phrase{En: "%s: 1 conversation, on %s.", De: "%s: ein Gespräch zu %s.", Vi: "%s: một cuộc trao đổi về %s."},
	ManyTitle: langcopy.Phrase{En: "%s: %d conversations, on %s.", De: "%s: %d Gespräche zu %s.", Vi: "%s: %d cuộc trao đổi về %s."},
}
