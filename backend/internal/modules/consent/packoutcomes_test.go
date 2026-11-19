// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// The outcomes the engine gives under the shipped packs. They are written as
// answers, not read off the rules. So a change to how a pack is built cannot
// move them unseen.

// shippedPackCodes registers each shipped pack under a test code of its own,
// handed out from shippedPackCodeRanges in the order of the real codes. The
// real country codes stay free for every other test in this binary.
func shippedPackCodes(t *testing.T) map[string]jurisdiction.Code {
	t.Helper()
	packs := shippedPacks(t)
	var free []jurisdiction.Code
	for _, r := range shippedPackCodeRanges {
		for second := r.first; second <= r.last; second++ {
			free = append(free, jurisdiction.Code(string([]rune{r.prefix, second})))
		}
	}
	countries := slices.Sorted(maps.Keys(packs))
	if len(countries) > len(free) {
		t.Fatalf("%d packs ship and shippedPackCodeRanges holds %d codes", len(countries), len(free))
	}
	out := map[string]jurisdiction.Code{}
	for i, country := range countries {
		out[country] = free[i]
	}
	registerShipped.Do(func() {
		for _, country := range countries {
			rules := packs[country]
			rules.Jurisdiction = out[country]
			messagingrules.Register(rules)
		}
	})
	return out
}

// shippedPackCodeRange is a prefix and the second letters handed out under it.
type shippedPackCodeRange struct{ prefix, first, last rune }

// shippedPackCodeRanges are the codes shippedPackCodes hands out, enough for
// the EU and Asia-Pacific units with room to grow. They sit outside the ranges
// testMessagingRules hands out, and
// gates/messagingtestcodes_test.go fails a literal code another test in this
// package registers inside them.
var shippedPackCodeRanges = [...]shippedPackCodeRange{
	{prefix: 'm', first: 'a', last: 'z'},
	{prefix: 'n', first: 'a', last: 'z'},
	{prefix: 'o', first: 'a', last: 'z'},
}

var registerShipped sync.Once

type packOutcome struct {
	reply, dealFollow time.Duration
	exception         *messagingrules.MarketingException
	unconsentedReason string
	capped            bool
	prefix            string
}

// existingCustomer is the Art. 13(2) exception with all four of its conditions.
var existingCustomer = &messagingrules.MarketingException{
	Kind: messagingrules.ExistingCustomer, RequiresSaleEvidence: true, RequiresCollectionTimeOptOut: true,
	RequiresSimilarity: true, RequiresNoObjection: true,
}

var shippedOutcomes = map[string]packOutcome{
	"de": {
		reply: 365 * 24 * time.Hour, dealFollow: 182 * 24 * time.Hour,
		exception:         existingCustomer,
		unconsentedReason: commsauthz.ReasonMarketingPermissionRequired,
	},
	"vn": {
		reply: 365 * 24 * time.Hour, dealFollow: 182 * 24 * time.Hour,
		unconsentedReason: commsauthz.ReasonMarketingConsentRequired,
		capped:            true, prefix: "[QC]",
	},
	"at": memberStateOutcome(false, ""), "be": memberStateOutcome(true, ""), "bg": memberStateOutcome(false, ""),
	"cy": memberStateOutcome(true, ""), "cz": memberStateOutcome(true, ""), "dk": memberStateOutcome(true, ""),
	"ee": memberStateOutcome(true, ""), "es": memberStateOutcome(true, "Publicidad"), "fi": memberStateOutcome(true, ""),
	"fr": memberStateOutcome(true, ""), "gr": memberStateOutcome(true, ""), "hr": memberStateOutcome(true, ""),
	"hu": memberStateOutcome(false, ""), "ie": memberStateOutcome(false, ""), "it": memberStateOutcome(true, ""),
	"lt": memberStateOutcome(true, ""), "lu": memberStateOutcome(true, ""), "lv": memberStateOutcome(true, ""),
	"mt": memberStateOutcome(true, ""), "nl": memberStateOutcome(true, ""), "pl": memberStateOutcome(false, ""),
	"pt": memberStateOutcome(true, ""), "ro": memberStateOutcome(true, ""), "se": memberStateOutcome(true, ""),
	"si": memberStateOutcome(true, ""), "sk": memberStateOutcome(true, ""),
	"au": apacOutcome(existingCustomer, commsauthz.ReasonMarketingPermissionRequired, ""),
	"nz": apacOutcome(existingCustomer, commsauthz.ReasonMarketingPermissionRequired, ""),
	"jp": apacOutcome(existingCustomer, commsauthz.ReasonMarketingPermissionRequired, ""),
	"kr": apacOutcome(nil, commsauthz.ReasonMarketingConsentRequired, "(광고)"),
	"cn": apacOutcome(nil, commsauthz.ReasonMarketingConsentRequired, "广告"),
	"sg": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, "<ADV>"),
	"hk": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"id": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"in": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"my": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"ph": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"th": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
	"tw": apacOutcome(nil, commsauthz.ReasonNoMarketingConsent, ""),
}

// apacOutcome is what an Asia-Pacific pack other than Vietnam's gives: the
// core windows, no cap, and its own exception, refusal and label.
func apacOutcome(exception *messagingrules.MarketingException, unconsented, prefix string) packOutcome {
	return packOutcome{
		reply: 365 * 24 * time.Hour, dealFollow: 182 * 24 * time.Hour,
		exception: exception, unconsentedReason: unconsented, prefix: prefix,
	}
}

// memberStateOutcome is what an EU member-state pack other than Germany's
// gives. With the existing-customer exception, advertising without consent
// asks for a permission; without it, the refusal stands as the core gave it.
func memberStateOutcome(exception bool, prefix string) packOutcome {
	out := packOutcome{
		reply: 365 * 24 * time.Hour, dealFollow: 182 * 24 * time.Hour,
		unconsentedReason: commsauthz.ReasonNoMarketingConsent, prefix: prefix,
	}
	if exception {
		out.exception = existingCustomer
		out.unconsentedReason = commsauthz.ReasonMarketingPermissionRequired
	}
	return out
}

func TestTheShippedPacksGiveTheOutcomesTheyGaveBeforeTheyWereCompiled(t *testing.T) {
	t.Parallel()
	codes := shippedPackCodes(t)
	if len(codes) != len(shippedOutcomes) {
		t.Fatalf("%d packs ship and %d have recorded outcomes", len(codes), len(shippedOutcomes))
	}
	for country, want := range shippedOutcomes {
		where := RecipientCountry{Code: codes[country], Source: CountryFromContactAddress}
		rules := packRulesFor(where)
		if rules.reply != want.reply || rules.dealFollow != want.dealFollow {
			t.Errorf("%s: windows %s/%s, want %s/%s", country, rules.reply, rules.dealFollow, want.reply, want.dealFollow)
		}
		if (rules.marketingException == nil) != (want.exception == nil) ||
			(want.exception != nil && *rules.marketingException != *want.exception) {
			t.Errorf("%s: exception %+v, want %+v", country, rules.marketingException, want.exception)
		}
		refused := commsauthz.Decision{Verdict: commsauthz.VerdictDeny, ReasonCode: commsauthz.ReasonNoMarketingConsent}
		if got := requireConsentWhereNothingStandsIn(refused, where).ReasonCode; got != want.unconsentedReason {
			t.Errorf("%s: advertising without consent is refused with %q, want %q", country, got, want.unconsentedReason)
		}
		applied, folded, _ := applicableRules(where.Code)
		if (applied.FrequencyCap != nil) != want.capped {
			t.Errorf("%s: capped = %v, want %v", country, applied.FrequencyCap != nil, want.capped)
		}
		unlabelled := subjectPrefixFindings(folded.SubjectPrefixes, "Offer")
		labelled := subjectPrefixFindings(folded.SubjectPrefixes, want.prefix+" Offer")
		if (len(unlabelled) > 0) != (want.prefix != "") || len(labelled) > 0 {
			t.Errorf("%s: an unlabelled subject gave %v and a labelled one %v", country, unlabelled, labelled)
		}
	}
}

// The draft and footer findings of representative sends, per pack.
func TestTheShippedPacksFlagTheDraftsTheyFlaggedBeforeTheyWereCompiled(t *testing.T) {
	t.Parallel()
	full := ControllerParticulars{
		LegalName: "Acme GmbH", PostalAddress: "Street 1", PrivacyContact: "privacy@acme.example",
		ObjectionRoute:    "https://acme.example/stop, stop@acme.example, +49 30 1234567",
		AdvertiserContact: "+49 30 1234567",
		Translations: []ParticularsTranslation{{
			Language: "vi", ObjectionRoute: "https://acme.example/dung, stop@acme.example, +84 24 1234 5678",
			AdvertiserContact: "+84 24 1234 5678",
		}},
	}
	for _, tc := range []struct {
		country, sender, subject, body string
		advertising                    bool
		particulars                    ControllerParticulars
		want                           []string
	}{
		{"de", "de", "Angebot", "Das beste Angebot.", true, full, nil},
		{
			"de", "de", "Angebot", "Hallo", true,
			ControllerParticulars{LegalName: "Acme GmbH"},
			[]string{"controller_identity", "objection_route", "privacy_contact"},
		},
		{
			"de", "de", "Rechnung", "Hallo", false,
			ControllerParticulars{LegalName: "Acme GmbH"},
			[]string{"controller_identity", "privacy_contact"},
		},
		{"de", "vn", "Angebot", "Hallo", true, ControllerParticulars{}, []string{"objection_route"}},
		{"vn", "vn", "[QC] Ưu đãi", vietnameseOffer, true, full, nil},
		{
			"vn", "de", "Offer", "This is the best offer.", true,
			ControllerParticulars{LegalName: "Acme GmbH"},
			// The advertiser's block carries the controller's identity, so no
			// line of its own is owed for it.
			[]string{
				"advertiser_contact", "advertising_language_first", "claim_without_proof",
				"foreign_advertiser", "objection_route", "privacy_contact", "subject_prefix",
			},
		},
		{"vn", "vn", "Hóa đơn", vietnameseOffer, false, ControllerParticulars{}, []string{"controller_identity", "privacy_contact"}},
	} {
		got := sendFindings(t, shippedSend{
			country: tc.country, sender: tc.sender, subject: tc.subject, body: tc.body,
			advertising: tc.advertising, particulars: tc.particulars, mailLanguage: "en",
		})
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s from %s, advertising %v, subject %q: findings %v, want %v",
				tc.country, tc.sender, tc.advertising, tc.subject, got, tc.want)
		}
	}
}

// shippedSend is one message to a recipient under a shipped pack.
type shippedSend struct {
	country, sender, subject, body string
	advertising                    bool
	particulars                    ControllerParticulars
	mailLanguage, language         string
}

// sendFindings answers the codes the engine's draft, subject and footer checks
// raise for one send. A sender in the recipient's own country, or a fixture
// that names no sender, sends under that pack; any other sender is established
// under no pack this test loads.
func sendFindings(t *testing.T, s shippedSend) []string {
	t.Helper()
	recipient := shippedPackCodes(t)[s.country]
	sender := jurisdiction.Code(s.sender)
	if s.sender == s.country || s.sender == "" {
		sender = recipient
	}
	var codes []string
	if s.advertising {
		for _, f := range packDraftFindings(recipient, sender, s.subject, s.body) {
			codes = append(codes, f.Requirement)
		}
		_, folded, _ := applicableRules(recipient)
		for _, f := range subjectPrefixFindings(folded.SubjectPrefixes, s.subject) {
			codes = append(codes, f.Requirement)
		}
	}
	owed, layout := owedDisclosures(sender, []jurisdiction.Code{recipient})
	layout.marketing, layout.language, layout.baseLanguage = s.advertising, s.language, s.mailLanguage
	for _, line := range layoutDisclosures(owed, s.particulars, layout) {
		if line.Missing() {
			codes = append(codes, line.Kind)
		}
	}
	slices.Sort(codes)
	return slices.Compact(codes)
}

// One subject begins with one label. A message under the Korean and the
// Singapore packs owes two, so it is held whatever its subject says.
func TestAMessageOwingTwoDifferentLabelsIsHeldUntilItsRecipientsAreSplit(t *testing.T) {
	t.Parallel()
	codes := shippedPackCodes(t)
	_, korean, _ := applicableRules(codes["kr"])
	if got := subjectPrefixFindings(korean.SubjectPrefixes, "(광고) Offer"); len(got) != 0 {
		t.Errorf("a labelled subject under one pack gave %v", got)
	}
	_, twice, _ := applicableRules(codes["kr"], codes["kr"])
	if got := subjectPrefixFindings(twice.SubjectPrefixes, "(광고) Offer"); len(got) != 0 {
		t.Errorf("a label owed twice gave %v", got)
	}
	_, both, _ := applicableRules(codes["kr"], codes["sg"])
	for _, subject := range []string{"(광고) Offer", "<ADV> Offer", "(광고) <ADV> Offer", "Offer"} {
		got := subjectPrefixFindings(both.SubjectPrefixes, subject)
		if len(got) != 1 || got[0].Requirement != RequirementSubjectPrefix || !strings.Contains(got[0].Detail, "split the recipients") {
			t.Errorf("%q under the Korean and Singapore labels gave %v, want one finding that asks for a split", subject, got)
		}
	}
}
