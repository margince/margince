// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import "github.com/margince/margince/backend/internal/platform/mailcopy"

func MailSubject(run BriefRun, words mailcopy.Copy) string {
	return words.MorningSubject + words.Date(run.LocalDay)
}
