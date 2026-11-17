// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SayWhy makes why the body of work Margince files: stored in language, and
// shown in each reader's own. A nil why leaves the body as it is.
func (in *LogActivityInput) SayWhy(why *systemtext.Component, language textlang.Lang) error {
	if why == nil {
		return nil
	}
	body, err := why.Render(language)
	if err != nil {
		return fmt.Errorf("activities: wording why the work exists: %w", err)
	}
	in.Body = &body
	if in.SystemText == nil {
		in.SystemText = &systemtext.Message{}
	}
	in.SystemText.Body = why
	return nil
}
