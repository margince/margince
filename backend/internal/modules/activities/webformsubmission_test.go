// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestOnlyAWebFormNoteAnswersItsRawAsASubmission(t *testing.T) {
	webForm, mailbox := provenance.WebFormSource, "gmail"
	raw := map[string]any{"email": "ada@visitor.example", "extra": []any{}, "campaign": []any{}}
	if got := webFormSubmissionOf(&webForm, raw); got == nil || got.Email != "ada@visitor.example" {
		t.Errorf("web form note = %+v, want its submission", got)
	}
	for name, c := range map[string]struct {
		system *string
		raw    map[string]any
	}{
		"another source":       {&mailbox, raw},
		"no source":            {nil, raw},
		"no raw":               {&webForm, nil},
		"a raw of other shape": {&webForm, map[string]any{"email": 42}},
	} {
		if got := webFormSubmissionOf(c.system, c.raw); got != nil {
			t.Errorf("%s: answered %+v, want nil", name, got)
		}
	}
}
