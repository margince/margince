// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

const (
	defaultRoute    = "Write to privacy@example.com, call +49 30 1234 5678 or open https://example.com/stop."
	vietnameseRoute = "gửi email đến privacy@example.com, gọi số +84 00 0000 0000 hoặc dùng trang https://example.com/stop"
	vietnameseReach = "Điện thoại +84 28 3822 9999, email hello@example.com, trang https://example.com"
)

// particularsWith states the default particulars in English and, when given,
// a Vietnamese objection route and advertiser contact.
func particularsWith(viRoute, viReach string) ControllerParticulars {
	p := ControllerParticulars{
		LegalName: "Example Trading GmbH", PostalAddress: "Hauptstraße 1\n10115 Berlin",
		PrivacyContact: "privacy@example.com", ObjectionRoute: defaultRoute,
		AdvertiserContact: "+49 30 1234 5678, hello@example.com, https://example.com",
	}
	if viRoute != "" || viReach != "" {
		p.Translations = []ParticularsTranslation{{
			Language: "vi", ObjectionRoute: viRoute, AdvertiserContact: viReach,
		}}
	}
	return p
}

func lineOf(lines []DisclosureLine, kind messagingrules.DisclosureKind) (DisclosureLine, bool) {
	for _, line := range lines {
		if line.Kind == string(kind) {
			return line, true
		}
	}
	return DisclosureLine{}, false
}

func TestAVietnameseMailPrintsTheVietnameseObjectionRouteWhenOneIsStored(t *testing.T) {
	owed := []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute}}
	layout := disclosureLayout{language: "vi", baseLanguage: "en"}

	stated, _ := lineOf(layoutDisclosures(owed, particularsWith(vietnameseRoute, ""), layout), messagingrules.ObjectionRoute)
	if stated.Text != vietnameseRoute {
		t.Errorf("a Vietnamese mail printed %q, want the stored Vietnamese route", stated.Text)
	}
	fallback, _ := lineOf(layoutDisclosures(owed, particularsWith("", ""), layout), messagingrules.ObjectionRoute)
	if fallback.Text != defaultRoute {
		t.Errorf("a Vietnamese mail with no Vietnamese route printed %q, want the default one", fallback.Text)
	}
	english, _ := lineOf(layoutDisclosures(owed, particularsWith(vietnameseRoute, ""),
		disclosureLayout{language: "en", baseLanguage: "en"}), messagingrules.ObjectionRoute)
	if english.Text != defaultRoute {
		t.Errorf("an English mail printed %q, want the default route", english.Text)
	}
}

// Advertising under a pack that names a disclosure language prints the
// particulars a reader acts on in that language. Otherwise they are missing,
// and the send is refused.
func TestAdvertisingInThePackLanguageNeedsTheParticularsInIt(t *testing.T) {
	owed := []messagingrules.Disclosure{
		{Kind: messagingrules.ObjectionRoute, MarketingOnly: true, Means: threeMeans},
		{Kind: messagingrules.AdvertiserContact, MarketingOnly: true},
	}
	layout := disclosureLayout{marketing: true, language: "en", baseLanguage: "en", packLanguage: "vi"}

	for _, line := range bothAdvertisingLines(t, layoutDisclosures(owed, particularsWith("", ""), layout)) {
		if !line.Missing() {
			t.Errorf("%s printed %q with no Vietnamese version stated", line.Kind, line.Text)
		}
	}
	for _, line := range bothAdvertisingLines(t, layoutDisclosures(owed, particularsWith(vietnameseRoute, vietnameseReach), layout)) {
		if line.Missing() {
			t.Errorf("%s is missing although its Vietnamese version is stated", line.Kind)
		}
	}
	// An installation that writes in the pack language has stated
	// its particulars in it already.
	native := disclosureLayout{marketing: true, language: "vi", baseLanguage: "vi", packLanguage: "vi"}
	for _, line := range bothAdvertisingLines(t, layoutDisclosures(owed, particularsWith("", ""), native)) {
		if line.Missing() {
			t.Errorf("%s is missing although the default text is written in the pack language", line.Kind)
		}
	}
}

// A route stated without one of the means its pack names does not meet the
// obligation, at send as at save.
func TestARouteWithoutAnAskedMeansIsMissingAtSend(t *testing.T) {
	owed := []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute, Means: threeMeans}}
	p := particularsWith("gửi email đến privacy@example.com hoặc dùng trang https://example.com/stop", "")
	line, _ := lineOf(layoutDisclosures(owed, p, disclosureLayout{language: "vi"}), messagingrules.ObjectionRoute)
	if !line.Missing() {
		t.Errorf("a route with no phone printed %q", line.Text)
	}
}

func TestAVietnameseAdvertisingMailEndsWithTheAdvertiserThenTheRefusal(t *testing.T) {
	owed := []messagingrules.Disclosure{
		{Kind: messagingrules.ControllerIdentity},
		{Kind: messagingrules.PrivacyContact},
		{Kind: messagingrules.ObjectionRoute, MarketingOnly: true, Means: threeMeans},
		{Kind: messagingrules.AdvertiserContact, MarketingOnly: true},
	}
	layout := disclosureLayout{
		marketing: true, language: "vi", baseLanguage: "en", packLanguage: "vi", refusalLast: true,
	}
	lines := layoutDisclosures(owed, particularsWith(vietnameseRoute, vietnameseReach), layout)

	kinds := make([]string, 0, len(lines))
	for _, line := range lines {
		kinds = append(kinds, line.Kind)
	}
	want := []string{
		string(messagingrules.PrivacyContact), string(messagingrules.AdvertiserContact),
		string(messagingrules.ObjectionRoute),
	}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("the lines run %v, want %v: the advertiser block directly before the refusal, at the end", kinds, want)
	}
	advertiser := lines[1].Text
	for _, part := range []string{"Example Trading GmbH", "Hauptstraße 1", vietnameseReach} {
		if !strings.Contains(advertiser, part) {
			t.Errorf("the advertiser block %q leaves out %q", advertiser, part)
		}
	}
	words := mailcopy.For("vi")
	refusal := lines[2].Text
	if !strings.HasPrefix(refusal, strings.Replace(words.AdvertisingRefusal, "%s", "Example Trading GmbH", 1)) ||
		!strings.Contains(refusal, vietnameseRoute) || !strings.HasSuffix(refusal, words.AdvertisingDataChoice) {
		t.Errorf("the refusal section reads %q, want the right to refuse, the route and the data choice", refusal)
	}
}

// Without an advertiser line to carry the name and address, the identity line
// stays.
func TestTheIdentityStaysWhenNoAdvertiserBlockCarriesIt(t *testing.T) {
	owed := []messagingrules.Disclosure{
		{Kind: messagingrules.ControllerIdentity},
		{Kind: messagingrules.ObjectionRoute, MarketingOnly: true},
	}
	lines := layoutDisclosures(owed, particularsWith("", ""), disclosureLayout{marketing: true, refusalLast: true})
	if _, ok := lineOf(lines, messagingrules.ControllerIdentity); !ok {
		t.Errorf("the identity line was dropped with no advertiser block to carry it: %+v", lines)
	}
}

// A further entity prints its own name and address in every language.
func TestAFurtherEntityKeepsItsOwnNameInATranslatedMail(t *testing.T) {
	installation := ControllerParticulars{
		LegalName: "Example Trading GmbH",
		Translations: []ParticularsTranslation{{
			Language: "vi", LegalName: "Công ty Ví dụ", ObjectionRoute: vietnameseRoute,
		}},
	}
	entity := sendingEntityFacts{found: true, legalName: "Example Vietnam LLC", address: "1 Lê Lợi\nHồ Chí Minh"}
	got := controllerOf(installation, entity).In("vi")
	if got.LegalName != "Example Vietnam LLC" || got.ObjectionRoute != vietnameseRoute {
		t.Errorf("the entity's Vietnamese mail names %q with route %q, want its own name and the stated route",
			got.LegalName, got.ObjectionRoute)
	}
}

// bothAdvertisingLines fails unless lines are the objection route and the
// advertiser contact, so a loop over them cannot pass on an empty answer.
func bothAdvertisingLines(t *testing.T, lines []DisclosureLine) []DisclosureLine {
	t.Helper()
	if len(lines) != 2 || lines[0].Kind != string(messagingrules.ObjectionRoute) ||
		lines[1].Kind != string(messagingrules.AdvertiserContact) {
		t.Fatalf("lines %+v, want the objection route and the advertiser contact", lines)
	}
	return lines
}
