// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"crypto/sha256"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Metadata is bounded independently: it must never prevent saving the message.
func boundedDraftEditorState(state crmcontracts.MailDraftEditorState) crmcontracts.MailDraftEditorState {
	for _, field := range []struct {
		value **string
		limit int
	}{
		{&state.Intent, 400}, {&state.AiDisclosure, 1000}, {&state.ServedBody, 20000},
	} {
		if *field.value != nil {
			*field.value = new(textcut.Runes(**field.value, field.limit))
		}
	}
	if state.VoiceProfileVersion != nil && *state.VoiceProfileVersion < 1 {
		state.VoiceProfileVersion = nil
	}
	if state.Language != nil && !textlang.Known(string(*state.Language)) {
		state.Language = nil
	}
	if state.Filing != nil {
		selection := make([]crmcontracts.MailDraftGround, 0, 2)
		seen := map[crmcontracts.MailDraftAnchorType]bool{}
		for _, ground := range *state.Filing {
			if ground.AnchorType != crmcontracts.MailDraftAnchorTypeDeal && ground.AnchorType != crmcontracts.MailDraftAnchorTypeProject {
				continue
			}
			if ground.AnchorId == (crmcontracts.Id{}) || seen[ground.AnchorType] {
				continue
			}
			selection = append(selection, ground)
			seen[ground.AnchorType] = true
			if len(selection) == 2 {
				break
			}
		}
		state.Filing = &selection
	}
	return state
}

// An older API can change the words without updating this column.
func draftContentFingerprint(content MailDraftContent) string {
	words := fmt.Sprintf("%q/%q/%q/%q/%q/%q", content.To, content.Cc, content.Bcc, content.Subject, content.Body, content.HTMLBody)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(words)))
}
