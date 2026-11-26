// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// activitySnippetRunes is the length the activity branch cuts its excerpt to,
// and the length a rendered excerpt is cut to, so the two compare.
const activitySnippetRunes = 200

// renderSystemText names an activity hit Margince wrote in the reader's
// language, the way its History row names it. The branch SQL titles a hit with
// the stored subject, which is English for every task Margince created.
//
// Only a hit still showing the stored words is changed. A title that fell
// through to the channel or the kind stays as ranked. So does a snippet whose
// body the descriptor no longer matches.
func renderSystemText(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	var activityIDs []ids.UUID
	for i := range hits {
		if hits[i].Type == hitTypeActivity {
			activityIDs = append(activityIDs, hits[i].ID)
		}
	}
	written, err := storekit.SystemWrittenActivities(ctx, tx, activityIDs)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	nameHits(hits, written, systemtext.Locale(ctx))
	return nil
}

// nameHits renders each hit whose words Margince wrote in the reader's language.
func nameHits(hits []Hit, written map[ids.UUID]storekit.SystemWords, reader textlang.Lang) {
	for i := range hits {
		words, ok := written[hits[i].ID]
		if hits[i].Type != hitTypeActivity || !ok {
			continue
		}
		if hits[i].Title == words.Subject {
			hits[i].Title = words.Message.SubjectIn(words.Subject, reader)
		}
		if hits[i].Snippet != "" && hits[i].Snippet == excerptOf(words.Body) {
			hits[i].Snippet = excerptOf(words.Message.BodyIn(words.Body, reader))
		}
	}
}

// excerptOf is a body cut the way the activity branch cuts it.
func excerptOf(body string) string {
	return strings.TrimSpace(textcut.Runes(body, activitySnippetRunes))
}
