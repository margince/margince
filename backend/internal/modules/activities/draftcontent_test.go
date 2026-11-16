// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// stubDraftChecker answers one finding and counts the drafts it was asked about.
type stubDraftChecker struct{ asked int }

func (c *stubDraftChecker) CheckAdvertisingDraft(context.Context, []string, string, string) ([]DraftContentFinding, error) {
	c.asked++
	return []DraftContentFinding{{Requirement: "claim_without_proof", Detail: "the draft claims"}}, nil
}

func decisionsResolvedTo(category commsauthz.Category) commsauthz.DecisionSet {
	return commsauthz.DecisionSet{Decisions: []commsauthz.Decision{{Resolved: category}}}
}

func TestAnAdvertisingDraftIsCheckedWhetherClaimedOrResolved(t *testing.T) {
	cases := []struct {
		name      string
		in        PreviewSendInput
		decisions commsauthz.DecisionSet
	}{
		{
			"claimed as marketing",
			PreviewSendInput{Context: commsauthz.CategoryMarketing, Body: "Ưu đãi"},
			decisionsResolvedTo(commsauthz.CategoryReplyToInbound),
		},
		{
			"resolved to marketing",
			PreviewSendInput{Body: "Ưu đãi"},
			decisionsResolvedTo(commsauthz.CategoryMarketing),
		},
	}
	for _, c := range cases {
		checker := &stubDraftChecker{}
		store := (&Store{}).WithDraftContent(checker)
		found, err := store.draftContentFindings(context.Background(), c.in, c.decisions)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if checker.asked != 1 || len(found) != 1 {
			t.Errorf("%s: the checker was asked %d times and answered %d findings, want 1 and 1",
				c.name, checker.asked, len(found))
		}
	}
}

func TestADraftThatIsNotAdvertisingOrHasNoTextIsNeverChecked(t *testing.T) {
	cases := []struct {
		name      string
		in        PreviewSendInput
		decisions commsauthz.DecisionSet
	}{
		{
			"a reply",
			PreviewSendInput{Context: commsauthz.CategoryReplyToInbound, Body: "Cảm ơn"},
			decisionsResolvedTo(commsauthz.CategoryReplyToInbound),
		},
		{
			"an empty draft",
			PreviewSendInput{Context: commsauthz.CategoryMarketing, Subject: " ", Body: "\n"},
			decisionsResolvedTo(commsauthz.CategoryMarketing),
		},
	}
	for _, c := range cases {
		checker := &stubDraftChecker{}
		store := (&Store{}).WithDraftContent(checker)
		found, err := store.draftContentFindings(context.Background(), c.in, c.decisions)
		if err != nil || checker.asked != 0 || len(found) != 0 {
			t.Errorf("%s: asked %d times, %d findings, err %v; want no check", c.name, checker.asked, len(found), err)
		}
	}
}
