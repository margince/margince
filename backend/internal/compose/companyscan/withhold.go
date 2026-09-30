// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Withholding: what happens to the WORDS a stored finding quotes when the
// reader stops being allowed to read them.
//
// The sibling of retraction next door, and the reason both exist is the same
// one: a finding is written once and replayed on every open, so it outlives the
// grants it was written under. Retraction answers the EXISTENCE question and
// drops the whole card. This answers the CONTENT question and keeps the card,
// because narrowing a message's audience does not make the record cease to
// exist — the reader may still legitimately know it is there.
//
// What it takes away is the part the narrowing was about. Evidence.Name is the
// message's subject line and Evidence.Quote is a verbatim extract of its body,
// both captured at scan time; EmailSummariesByIDBatch already refuses this
// reader the live summary, so the citation is unpressable while the words it
// was pressed for sit above it on the card.

// withholdQuotedWords blanks the subject and the verbatim quote on every
// activity citation whose words this reader may no longer read.
//
// Over the MERGED list rather than the stored half alone. The rules' live
// advice is assembled under the reader's own scope and so passes this check by
// construction, and asking about it costs nothing — where a check that trusted
// one producer would be a rule the next producer has to remember.
//
// The finding survives with its words gone rather than being dropped: an
// audience narrowing is not a retraction, and deleting advice about a record
// the reader may still open would lose guidance they are entitled to.
func (s *Service) withholdQuotedWords(
	ctx context.Context, findings []crmcontracts.Company360Suggestion,
) error {
	cited := citedActivities(findings)
	if len(cited) == 0 {
		return nil
	}
	var readable map[ids.UUID]bool
	if err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var readErr error
		readable, readErr = activities.ReadableActivityContent(ctx, tx, cited)
		return readErr
	}); err != nil {
		return err
	}
	blankUnreadable(findings, readable)
	return nil
}

// blankUnreadable clears the words on the citations the set does not admit.
//
// Only the activity arm: a deal's name and a signal's sentence are reached by
// their own records' gates, and blanking them on an activity's answer would
// withhold evidence nobody narrowed.
func blankUnreadable(findings []crmcontracts.Company360Suggestion, readable map[ids.UUID]bool) {
	for i := range findings {
		evidence := findings[i].Evidence
		for j := range evidence {
			row := &evidence[j]
			if row.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
				continue
			}
			if readable[ids.UUID(row.EntityId)] {
				continue
			}
			row.Name = nil
			row.Quote = nil
		}
	}
}
