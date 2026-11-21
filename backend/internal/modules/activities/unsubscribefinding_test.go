// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// holderlessLinker finds no holder for any address, so it can give no send a
// stop link.
type holderlessLinker struct{ asked []string }

func (l *holderlessLinker) UnsubscribeToken(_ context.Context, address, _ string) (string, string, bool, error) {
	l.asked = append(l.asked, address)
	return "", "", false, nil
}

func (l *holderlessLinker) ManageToken(context.Context, string) (string, bool, error) {
	return "", false, nil
}

// A send that owes a stop link and gets none records the missing link, which
// holds it for a Send anyway approval. The transactional purpose owes none, so
// the same missing link records nothing and holds nothing.
func TestOnlyASendThatOwesAStopLinkRecordsItsAbsence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		surface  unsubscribeSurface
		recorded bool
	}{
		{"marketing", surfaceFor(commsauthz.CategoryMarketing, "newsletter", ""), true},
		{"legacy marketing key", surfaceFor("", "", "newsletter"), true},
		{"transactional key", surfaceFor("", "", "transactional"), false},
		{"marketing on the transactional key", surfaceFor(commsauthz.CategoryMarketing, "", "transactional"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			linker := &holderlessLinker{}
			store := (&Store{}).WithUnsubscribe(linker).WithPublicBaseURL("https://crm.example.test")
			delivered, err := store.deliverability(context.Background(), "Message", "Subject", []string{"buyer@example.test"}, nil, tc.surface, ids.Nil, "")
			if err != nil {
				t.Fatal(err)
			}
			recorded := false
			for _, finding := range delivered.footerFindings {
				recorded = recorded || finding.Kind == footerKindUnsubscribeLink
			}
			if recorded != tc.recorded {
				t.Fatalf("missing stop link recorded = %v, want %v (findings %+v)", recorded, tc.recorded, delivered.footerFindings)
			}
			held := commsauthz.Decision{Verdict: commsauthz.VerdictAllow}.WithFooterFindings(delivered.footerFindings).Verdict != commsauthz.VerdictAllow
			if held != tc.recorded {
				t.Fatalf("held for a Send anyway approval = %v, want %v", held, tc.recorded)
			}
		})
	}
}

// One addressee listed twice, in two spellings, is asked about once and by
// the address a mail server reads.
func TestTheLinkerIsAskedAboutTheOneNormalizedAddress(t *testing.T) {
	linker := &holderlessLinker{}
	store := (&Store{}).WithUnsubscribe(linker).WithPublicBaseURL("https://crm.example.test")
	recipients := []string{"  Buyer@Example.test ", "buyer@example.test"}
	if _, err := store.deliverability(context.Background(), "Message", "Subject", recipients, nil, surfaceFor(commsauthz.CategoryMarketing, "newsletter", ""), ids.Nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(linker.asked) != 1 || linker.asked[0] != "buyer@example.test" {
		t.Fatalf("the linker was asked about %q, want the one normalized address", linker.asked)
	}
}
