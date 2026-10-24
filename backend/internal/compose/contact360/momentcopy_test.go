// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"reflect"
	"strings"
	"testing"
	"time"

	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestContactMomentCopyCensus(t *testing.T) {
	langcopytest.Census(t, momentWords)
	langcopytest.NoCount(t, map[string]langcopy.Phrase{"one day ago": momentWords.PromisedYesterday})
}

func TestEveryContactMomentRungKeepsItsEvidenceAndActionAcrossLanguages(t *testing.T) {
	pages := momentPages()
	pages["unnamed task"] = &contracts.Contact360{NextSteps: timelineOf(contracts.Activity{Kind: "task", OccurredAt: now, DueAt: new(ahead(24))})}
	for _, lang := range textlang.Shipped {
		said := momentCopyFor(string(lang))
		fired := make(map[int]bool)
		for name, page := range pages {
			original := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
			translated := said.deriveMoment(readerCtx(), now, page)
			checkMomentLanguage(t, string(lang), name, original, translated)
			for i, rung := range said.ladder() {
				got, ok := rung(readerCtx(), now, page)
				want, englishFired := momentCopyFor("en").ladder()[i](readerCtx(), now, page)
				if ok != englishFired {
					t.Fatalf("%s changed whether rung %d fires", lang, i)
				}
				if !ok {
					continue
				}
				fired[i] = true
				if string(got.Rule) != momentLadderNames[i] {
					t.Fatalf("rung %d returned %s, want %s", i, got.Rule, momentLadderNames[i])
				}
				if !englishFired {
					t.Fatalf("%s added a rung", lang)
				}
				checkMomentLanguage(t, string(lang), name, want, got)
			}
		}
		if len(fired) != len(said.ladder()) {
			t.Fatalf("%s exercised %d of %d rungs", lang, len(fired), len(said.ladder()))
		}
	}
}

func checkMomentLanguage(t *testing.T, language, name string, original, translated contracts.ContactMoment) {
	t.Helper()
	if language != "en" && (translated.Headline == original.Headline || translated.WhyNow == original.WhyNow || translated.RecommendedAction.Label == original.RecommendedAction.Label) {
		t.Fatalf("%s/%s retained English copy: %+v", language, name, translated)
	}
	if strings.Contains(translated.Headline+translated.WhyNow, "%!") {
		t.Fatal("unbound format parameter")
	}

	if original.SecondaryActions != nil && translated.SecondaryActions != nil {
		for i, action := range *translated.SecondaryActions {
			before := (*original.SecondaryActions)[i]
			if language != "en" && action.Label == before.Label {
				t.Fatalf("%s secondary action stayed English", language)
			}
			if strings.Contains(action.Label, "%!") {
				t.Fatal("secondary action has unbound format")
			}
			if action.BlockedReason != nil && (strings.Contains(*action.BlockedReason, "%!") || (language != "en" && before.BlockedReason != nil && *action.BlockedReason == *before.BlockedReason)) {
				t.Fatalf("%s invalid secondary reason", language)
			}
		}
	}
	for i, evidence := range translated.Evidence {
		if strings.Contains(evidence.Label, "%!") {
			t.Fatal("evidence caption has unbound format")
		}
		if evidence.Snippet != nil && evidence.Label != original.Evidence[i].Label {
			t.Fatal("quoted claim label changed")
		}
	}
	if !reflect.DeepEqual(momentFacts(original), momentFacts(translated)) {
		t.Fatalf("%s/%s changed card facts or action: %+v / %+v", language, name, original, translated)
	}
}

func momentFacts(moment contracts.ContactMoment) contracts.ContactMoment {
	moment.Headline = ""
	moment.WhyNow = ""
	moment.RecommendedAction.Label = ""
	moment.RecommendedAction.BlockedReason = nil
	if moment.SecondaryActions != nil {
		for i := range *moment.SecondaryActions {
			(*moment.SecondaryActions)[i].Label = ""
			(*moment.SecondaryActions)[i].BlockedReason = nil
		}
	}
	for i := range moment.Evidence {
		moment.Evidence[i].Label = ""
	}
	return moment
}

func TestUnknownContactRoleDoesNotBecomeARecordedRoleInTranslation(t *testing.T) {
	for _, language := range []string{"de", "vi"} {
		said := momentCopyFor(language)
		for _, role := range []*string{nil, new(""), new("  "), new("economic_buyer")} {
			got := said.recordedSeat(role)
			recorded := "erfasste Rolle"
			if language == "vi" {
				recorded = "được ghi nhận"
			}
			wantRecorded := role != nil && strings.TrimSpace(*role) != ""
			if strings.Contains(got, recorded) != wantRecorded {
				t.Fatalf("%s inferred or lost role: %s", language, got)
			}
		}
	}
}

func TestUnnamedTaskNeverInventsAnEmailSubject(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		task := contracts.Activity{Kind: "task", OccurredAt: now}
		got := momentCopyFor(language).openPromiseFrom(readerCtx(), now, task)
		if got.RecommendedAction.Destination == nil || got.RecommendedAction.Destination.Prefill == nil {
			t.Fatal("composer intent lost")
		}
		if (*got.RecommendedAction.Destination.Prefill)[prefillSubject] != "" {
			t.Fatalf("%s invented a subject", language)
		}
		if got.Headline == "" || len(got.Evidence) != 1 || got.Evidence[0].Label == "" {
			t.Fatal("unnamed task lost display caption")
		}
	}
}

func TestUndatedTaskUsesTheRecordDayInEveryLanguage(t *testing.T) {
	task := contracts.Activity{Kind: "task", OccurredAt: time.Date(2026, 8, 3, 20, 0, 0, 0, time.UTC)}
	for _, language := range []string{"en", "de", "vi"} {
		said := momentCopyFor(language)
		said.zone = time.FixedZone("UTC+7", 7*60*60)
		if got := said.openPromiseWhyNow(now, task); !strings.Contains(got, "2026-08-04") {
			t.Fatalf("%s used another day: %s", language, got)
		}
	}
}

func TestContactEvidenceFallbackCaptionsFollowLanguage(t *testing.T) {
	for _, tc := range []struct{ lang, inbound, outbound, meeting string }{
		{"en", "Their last message", "Last message to them", "Meeting"},
		{"de", "Letzte Nachricht des Kontakts", "Letzte Nachricht an den Kontakt", "die Besprechung"},
		{"vi", "Tin nhắn gần nhất của họ", "Tin nhắn gần nhất gửi cho họ", "Cuộc họp"},
	} {
		said := momentCopyFor(tc.lang)
		if got := said.inboundEvidence(&contracts.Contact360{}, now).Label; got != tc.inbound {
			t.Errorf("inbound %s: %s", tc.lang, got)
		}
		if got := said.outboundEvidence(&contracts.Contact360{}, now)[0].Label; got != tc.outbound {
			t.Errorf("outbound %s: %s", tc.lang, got)
		}
		meeting, ok := said.meetingPrepMoment(readerCtx(), now, &contracts.Contact360{NextMeeting: &contracts.Contact360NextMeeting{StartsAt: ahead(24)}})
		if !ok || meeting.Evidence[0].Label != tc.meeting {
			t.Fatalf("%s meeting fallback: %+v", tc.lang, meeting)
		}
	}
}

// Every colleague who opens the contact reads the gone-quiet copy. Most of
// them sent nothing, so it never says the reader wrote the last message.
func TestTheGoneQuietCopyNeverClaimsTheReaderSentTheLastMessage(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		said := momentCopyFor(language)
		outbound := now.AddDate(0, 0, -40)
		moment, ok := said.goneQuietMoment(readerCtx(), now, &contracts.Contact360{LastOutboundAt: &outbound})
		if !ok {
			t.Fatalf("%s: no gone-quiet moment for a 40-day silence", language)
		}
		for _, claim := range []string{"Your last", "Ihre letzte", "của bạn"} {
			if strings.Contains(moment.WhyNow, claim) || strings.Contains(moment.Evidence[0].Label, claim) {
				t.Errorf("%s: %q / %q says the reader sent it", language, moment.WhyNow, moment.Evidence[0].Label)
			}
		}
	}
}
