// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// languageInsertFragment stamps the language a hand-logged note, meeting or
// call is written in. A text the detector cannot place stores no label, the
// same answer capture gives.
func languageInsertFragment(in LogActivityInput, base []any) (col, holder string, args []any) {
	if !stampsLanguage(in.Kind) {
		return "", "", base
	}
	lang := textlang.OfMessage(deref(in.Body), deref(in.Subject))
	if lang == textlang.Unknown {
		return "", "", base
	}
	args = append(append(make([]any, 0, len(base)+1), base...), string(lang))
	return ", language", fmt.Sprintf(", $%d", len(args)), args
}

// stampsLanguage names the kinds a member writes prose into by hand. A logged
// email keeps the label its capture gave it.
func stampsLanguage(kind string) bool {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindNote, crmcontracts.ActivityKindMeeting, crmcontracts.ActivityKindCall:
		return true
	default:
		return false
	}
}

// relabelledLanguage is the label an edit leaves, read from the text as it
// stands after the edit. Nil when the detector cannot place it, which clears
// a label the old words had earned.
func relabelledLanguage(current crmcontracts.Activity, in UpdateActivityInput) *string {
	subject, body := current.Subject, current.Body
	if in.Subject != nil {
		subject = in.Subject
	}
	if in.Body != nil {
		body = in.Body
	}
	lang := textlang.OfMessage(deref(body), deref(subject))
	if lang == textlang.Unknown {
		return nil
	}
	label := string(lang)
	return &label
}
