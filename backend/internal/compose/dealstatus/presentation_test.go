// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"reflect"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestFixedMoveCopyUsesTheReaderWhileTheStoredCardStaysCanonical(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, lang: "en", openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Send pricing"}}}
	canonical := decideMove(f)
	card := crmcontracts.DealStatusCard{Next: &canonical}
	english := presentMove(systemtext.WithLocale(context.Background(), textlang.English), f, card)
	vietnamese := presentMove(systemtext.WithLocale(context.Background(), textlang.Vietnamese), f, card)
	if english.Next.Reason != "Complete the existing task: Send pricing" {
		t.Fatalf("English=%q", english.Next.Reason)
	}
	if vietnamese.Next.Reason != "Hoàn thành nhiệm vụ hiện có: Send pricing" {
		t.Fatalf("Vietnamese=%q", vietnamese.Next.Reason)
	}
	if card.Next.Reason != english.Next.Reason {
		t.Fatal("presentation mutated the stored move")
	}
	if *vietnamese.Next.Fingerprint != *english.Next.Fingerprint {
		t.Fatal("reader language split the move identity")
	}
}

func TestLocalizedTaskArgumentsKeepTheirAssessedRecommendation(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, lang: "en"}
	canonical, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	if canonical.Action != ActionCreateTask || canonical.Arguments == nil {
		t.Fatalf("fixture has no proposed task: %+v", canonical)
	}
	for _, locale := range []textlang.Lang{textlang.English, textlang.Vietnamese} {
		card := presentMove(systemtext.WithLocale(context.Background(), locale), f, crmcontracts.DealStatusCard{Next: &canonical})
		if card.Next.Arguments == nil || !reflect.DeepEqual((*card.Next.Arguments)["recommendation"], (*canonical.Arguments)["recommendation"]) {
			t.Fatal("localized task lost recommendation provenance")
		}
	}
}

func TestReaderWordingKeepsTheMoveAndItsRecommendationBasis(t *testing.T) {
	closed := openDeal()
	closed.Status = crmcontracts.DealStatusWon
	for _, f := range []facts{
		{deal: openDeal()},
		{deal: closed},
		{deal: openDeal(), openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Send pricing"}}},
		{deal: openDeal(), nextMeeting: booked(testNow.Add(time.Hour))},
		{deal: openDeal(), requests: []crmcontracts.Activity{inboundMail(testNow.Add(-time.Hour))}},
	} {
		f.now, f.loc, f.lang = testNow, time.UTC, "en"
		canonical, err := assessMove(f, decideMove(f))
		if err != nil {
			t.Fatal(err)
		}
		for _, language := range []string{"de", "vi"} {
			f.say = language
			translated, err := assessMove(f, decideMove(f))
			if err != nil {
				t.Fatal(err)
			}
			if translated.Action != canonical.Action || !reflect.DeepEqual(translated.Fingerprint, canonical.Fingerprint) || !reflect.DeepEqual(translated.Basis, canonical.Basis) || !reflect.DeepEqual(translated.Recommendation, canonical.Recommendation) {
				t.Fatalf("reader language %s changed the %s move identity or basis", language, canonical.Action)
			}
		}
	}
}
