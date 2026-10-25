// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An edition is stored for every reader, so its captions take the installation's
// language. The owner's own live read takes the owner's language.
func TestAnEditionIsWrittenInTheBaseLanguageWhileTheOwnersLiveReadIsInTheirs(t *testing.T) {
	q := newQuestionEditions(t)
	owner, _ := principal.Actor(q.human)
	if _, err := q.e.Pool.Exec(context.Background(), `UPDATE app_user SET locale = 'de' WHERE id = $1`, owner.UserID); err != nil {
		t.Fatal(err)
	}

	edition := q.send(t)
	if got := edition.Evaluation.Context.ProseLanguage; got == nil || *got != "en" {
		t.Fatalf("the stored edition is written in %v, want the base language en", proseLanguageName(got))
	}

	live, err := q.service.Evaluate(q.human, q.report.Selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := live.Context.ProseLanguage; got == nil || *got != "de" {
		t.Fatalf("the owner's live read is written in %v, want de", proseLanguageName(got))
	}
}

func proseLanguageName(prose *string) string {
	if prose == nil {
		return "(none)"
	}
	return *prose
}
