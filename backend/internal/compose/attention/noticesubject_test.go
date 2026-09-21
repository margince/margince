// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A notice that names a record says WHICH one, so the Worklist row links to it.
//
// "A deal you own changed stage" is true of every deal a rep owns. The row
// carried the sentence and nothing else, so a reader knew something had moved
// and had to go find it.

func unread(targetType string, targetID ids.UUID) UnreadNotice {
	return UnreadNotice{
		ID:         ids.NewV7(),
		Kind:       "automation",
		Subject:    "A deal you own changed stage",
		Body:       "Fleet retrofit moved to a new pipeline stage.",
		TargetType: targetType,
		TargetID:   targetID,
		CreatedAt:  time.Date(2026, time.September, 8, 9, 0, 0, 0, time.UTC),
	}
}

func TestANoticeRowNamesTheRecordItIsAbout(t *testing.T) {
	t.Parallel()
	deal := ids.NewV7()

	item := noticeItem(unread("deal", deal))

	if item.Subject == nil {
		t.Fatal("the row names no record, so it links nowhere and the reader has to go find it")
	}
	if item.Subject.Type != crmcontracts.AttentionSubjectTypeDeal {
		t.Errorf("subject type = %q, want deal", item.Subject.Type)
	}
	if ids.UUID(item.Subject.Id) != deal {
		t.Errorf("subject names %s, want the deal the notice is about (%s)", item.Subject.Id, deal)
	}
}

// The refusal case, and the reason the admit case above means something: a
// notice about no record carries no subject rather than one pointing at a zero.
func TestANoticeAboutNoRecordNamesNone(t *testing.T) {
	t.Parallel()
	if item := noticeItem(unread("", ids.UUID{})); item.Subject != nil {
		t.Errorf("a notice about no record named %+v", *item.Subject)
	}
}

// A type this feed cannot route to yields no subject either. The client turns
// a subject into a link, so a subject it has no screen for is a control that
// opens nothing — worse than a row that never offered one.
func TestANoticeNamingAnUnroutableTypeNamesNone(t *testing.T) {
	t.Parallel()
	if item := noticeItem(unread("invoice", ids.NewV7())); item.Subject != nil {
		t.Errorf("an unroutable target became a subject: %+v", *item.Subject)
	}
}

// The row's own words survive the subject arriving beside them: the headline
// and the detail are what a reader scans, and the link is one more thing the
// row offers rather than a replacement for either.
func TestANoticeRowKeepsItsWordsBesideTheLink(t *testing.T) {
	t.Parallel()
	notice := unread("deal", ids.NewV7())

	item := noticeItem(notice)

	if item.Title == nil || *item.Title != notice.Subject {
		t.Errorf("title = %v, want the notice's own subject", item.Title)
	}
	if item.Detail == nil || *item.Detail != notice.Body {
		t.Errorf("detail = %v, want the notice's own body", item.Detail)
	}
}
