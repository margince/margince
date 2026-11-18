// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What the disclosure footer puts in a body, and what it leaves out.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestAStatedDisclosureReachesTheBody is the register line this closes: the
// obligations were declared, rendered, and put nowhere.
func TestAStatedDisclosureReachesTheBody(t *testing.T) {
	body := appendDisclosures("As discussed.", []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH\nHauptstraße 1"},
		{Kind: "privacy_contact", Text: "datenschutz@beispiel.test"},
	})

	if !strings.Contains(body, "Beispiel GmbH") {
		t.Errorf("the controller identity is not in the body:\n%s", body)
	}
	if !strings.Contains(body, "datenschutz@beispiel.test") {
		t.Errorf("the privacy contact is not in the body:\n%s", body)
	}
	if !strings.HasPrefix(body, "As discussed.") {
		t.Errorf("the footer displaced the message:\n%s", body)
	}
}

// TestAnUnmeetableDisclosureIsLeftOutOfTheBody.
//
// A line the installation cannot meet is reported to the OPERATOR and kept out
// of the message. Printing a gap where the controller's name belongs discloses
// nothing and reads to the recipient as a defect — and the operator is who can
// actually fix it.
func TestAnUnmeetableDisclosureIsLeftOutOfTheBody(t *testing.T) {
	body := appendDisclosures("As discussed.", []DisclosureLine{
		{Kind: "controller_identity", Text: ""},
		{Kind: "privacy_contact", Text: "datenschutz@beispiel.test"},
	})

	if strings.Contains(body, "controller_identity") {
		t.Errorf("the unmet obligation's NAME reached the recipient:\n%s", body)
	}
	if !strings.Contains(body, "datenschutz@beispiel.test") {
		t.Errorf("the obligation that could be met was dropped with the one that could not:\n%s",
			body)
	}
}

// TestNothingOwedLeavesTheBodyExactlyAsItWas. An installation with no
// applicable pack sends what it always sent, byte for byte — a footer separator
// on a message owing nothing would be a visible change for every installation
// outside the jurisdictions that have packs.
func TestNothingOwedLeavesTheBodyExactlyAsItWas(t *testing.T) {
	const original = "As discussed."
	for _, lines := range [][]DisclosureLine{
		nil,
		{},
	} {
		if got := appendDisclosures(original, lines); got != original {
			t.Errorf("a message owing nothing came back as %q, want it unchanged", got)
		}
	}
}

// TestAnObligationTheWorkspaceCannotMeetIsNotTheSameAsNoObligation. These two
// were folded together once: an unmeetable line rendered as empty and the body
// came back unchanged, which is byte-identical to a jurisdiction demanding
// nothing. The first is a compliance gap somebody has to close.
func TestAnObligationTheWorkspaceCannotMeetIsNotTheSameAsNoObligation(t *testing.T) {
	for _, line := range []DisclosureLine{
		{Kind: "controller_identity", Text: ""},
		{Kind: "controller_identity", Text: "   "},
	} {
		if !line.Missing() {
			t.Errorf("%+v reports itself meetable, want missing", line)
		}
		if got := unmeetable([]DisclosureLine{line}); len(got) != 1 || got[0].Kind != "controller_identity" {
			t.Errorf("unmeetable(%+v) = %v, want [controller_identity]", line, got)
		}
	}
	if got := unmeetable([]DisclosureLine{{Kind: "controller_identity", Text: "Beispiel GmbH"}}); len(got) != 0 {
		t.Errorf("a stated particular reported unmeetable: %v", got)
	}
}

// TestTheFooterIsSeparatedFromTheMessage. A disclosure run together with the
// last sentence reads as part of what the sender wrote.
func TestTheFooterIsSeparatedFromTheMessage(t *testing.T) {
	body := appendDisclosures("As discussed.", []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH"},
	})
	if !strings.Contains(body, "\n\n--\n") {
		t.Errorf("the disclosures are not separated from the message:\n%s", body)
	}
}

// stubDisclosures answers a fixed set, for the wiring tests below.
type stubDisclosures struct {
	lines    []DisclosureLine
	asked    []string
	entities []ids.UUID
}

func (s *stubDisclosures) DisclosuresFor(_ context.Context, category string, _ []string, entity ids.UUID, _ string) ([]DisclosureLine, error) {
	s.asked = append(s.asked, category)
	s.entities = append(s.entities, entity)
	return s.lines, nil
}

// TestADisclosureRidesAMessageCarryingNoUnsubscribeSurface is the reason this
// is a separate footer rather than part of the unsubscribe one.
//
// An unsubscribe surface belongs to advertising. A controller identity binds a
// first contact whatever it is about, so a disclosure appended inside the
// marketing arm would appear on advertising and be absent from exactly the
// correspondence Art. 13 covers.
//
// Correspondence carries no unsubscribe surface, so deliverability returns
// early — and the disclosure must already be in the body by then.
func TestADisclosureRidesAMessageCarryingNoUnsubscribeSurface(t *testing.T) {
	stub := &stubDisclosures{lines: []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH"},
	}}
	store := (&Store{}).WithDisclosures(stub)

	got, err := store.deliverability(context.Background(), "As discussed.", "Re: your question",
		[]string{"anna@example.test"},
		unsubscribeSurface{category: commsauthz.CategoryReplyToInbound}, ids.Nil, "")
	if err != nil {
		t.Fatalf("deriving the message: %v", err)
	}
	if !strings.Contains(got.transmitted, "Beispiel GmbH") {
		t.Errorf("a reply went out with no controller identity:\n%s\n\nThe unsubscribe "+
			"surface belongs to advertising and this message has none, so a disclosure "+
			"appended inside that arm never reaches the correspondence Art. 13 covers",
			got.transmitted)
	}
	if !strings.Contains(got.recorded, "Beispiel GmbH") {
		t.Errorf("the recorded copy differs from what was sent:\n%s", got.recorded)
	}
}

// TestADisclosureSurvivesEveryEarlyReturn. A send with no linker wired and one
// with no recipients both return before the marketing arm, and both still owe
// their disclosures.
func TestADisclosureSurvivesEveryEarlyReturn(t *testing.T) {
	stub := &stubDisclosures{lines: []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH"},
	}}
	store := (&Store{}).WithDisclosures(stub)

	for _, tc := range []struct {
		name       string
		recipients []string
	}{
		{"no linker wired", []string{"anna@example.test"}},
		{"no recipients", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.deliverability(context.Background(), "As discussed.", "Subject",
				tc.recipients, unsubscribeSurface{category: commsauthz.CategoryMarketing}, ids.Nil, "")
			if err != nil {
				t.Fatalf("deriving the message: %v", err)
			}
			if !strings.Contains(got.transmitted, "Beispiel GmbH") {
				t.Errorf("the disclosure was lost on an early return:\n%s", got.transmitted)
			}
		})
	}
}

// TestTheCategoryCrossesTheSeam. The resolver decides which obligations apply
// from the category, so a send that handed it the wrong one would render
// somebody else's rules.
func TestTheCategoryCrossesTheSeam(t *testing.T) {
	stub := &stubDisclosures{}
	store := (&Store{}).WithDisclosures(stub)

	if _, err := store.deliverability(context.Background(), "body", "subject",
		[]string{"anna@example.test"},
		unsubscribeSurface{category: commsauthz.CategoryMarketing}, ids.Nil, ""); err != nil {
		t.Fatalf("deriving the message: %v", err)
	}
	if len(stub.asked) != 1 || stub.asked[0] != string(commsauthz.CategoryMarketing) {
		t.Errorf("the resolver was asked about %v, want the message's own category", stub.asked)
	}
}

// TestNoResolverWiredSendsWhatItAlwaysSent. Every test store in this package
// composes without one, and none of them should have to learn what a
// jurisdiction pack is. Production wires it on both doors.
func TestNoResolverWiredSendsWhatItAlwaysSent(t *testing.T) {
	got, err := (&Store{}).deliverability(context.Background(), "As discussed.", "Subject",
		[]string{"anna@example.test"},
		unsubscribeSurface{category: commsauthz.CategoryMarketing}, ids.Nil, "")
	if err != nil {
		t.Fatalf("deriving the message: %v", err)
	}
	if got.transmitted != "As discussed." {
		t.Errorf("a store with no resolver changed the body to %q", got.transmitted)
	}
}

// TestTheMarkupAlternativeCarriesTheSameDisclosures is the defect Codex found.
//
// Which alternative a recipient reads is their mail client's decision. A
// message whose HTML omitted its controller identity would be compliant only
// for readers whose client fell back to text — and the recorded copy, which is
// the text one, would look compliant to every later reader of the timeline.
func TestTheMarkupAlternativeCarriesTheSameDisclosures(t *testing.T) {
	stub := &stubDisclosures{lines: []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH\nHauptstraße 1"},
	}}
	store := (&Store{}).WithDisclosures(stub)

	// Through signedHTML, which is what sendcore.go calls — not the helper it
	// happens to use. A test on the helper alone passes whether or not the
	// markup path is wired, which is exactly the gap that let the HTML
	// alternative ship without disclosures in the first place.
	derived, err := store.deliverability(context.Background(), "As discussed.", "Subject",
		[]string{"anna@example.test"},
		unsubscribeSurface{category: commsauthz.CategoryReplyToInbound}, ids.Nil, "")
	if err != nil {
		t.Fatalf("deriving the message: %v", err)
	}
	got := signedHTML("<p>As discussed.</p>", SignOff{Kind: SignOffNone}, derived)

	if !strings.Contains(got, "Beispiel GmbH") {
		t.Errorf("the markup alternative carries no controller identity:\n%s\n\nWhich "+
			"alternative a recipient reads is their client's decision, and the recorded "+
			"copy is the text one — so this is compliant only for readers whose client "+
			"fell back to text", got)
	}
	// The line break survives as markup, the way the signature's does: a name
	// and an address written on two lines are two lines.
	if !strings.Contains(got, "<br>") {
		t.Errorf("the address lost its line break in markup: %q", got)
	}
}

// TestTheMarkupAlternativeEscapesWhatAnOperatorTyped. These are words from a
// settings form about to become markup.
func TestTheMarkupAlternativeEscapesWhatAnOperatorTyped(t *testing.T) {
	got := htmlDisclosures([]DisclosureLine{
		{Kind: "controller_identity", Text: `Beispiel <script>alert(1)</script> GmbH`},
	})
	if strings.Contains(got, "<script>") {
		t.Errorf("an operator's text reached the markup unescaped: %q", got)
	}
}

// TestNothingOwedAddsNoMarkup. An installation with no applicable pack sends
// the HTML it always sent.
func TestNothingOwedAddsNoMarkup(t *testing.T) {
	if got := htmlDisclosures(nil); got != "" {
		t.Errorf("a message owing nothing gained markup: %q", got)
	}
	if got := htmlDisclosures([]DisclosureLine{{Kind: "x", Text: "  "}}); got != "" {
		t.Errorf("an unmeetable obligation gained markup: %q", got)
	}
}

// TestALegacyMarketingSendKeepsItsMarketingOnlyDisclosures.
//
// A caller naming only the deprecated consent key carries no category, and
// asking the resolver about an empty one drops every marketing-only obligation
// — the German objection route, the Vietnamese advertiser contact — from
// exactly the sends that carry an unsubscribe surface and are therefore
// advertising by this product's own reckoning.
func TestALegacyMarketingSendKeepsItsMarketingOnlyDisclosures(t *testing.T) {
	surface := surfaceFor("", "", "marketing_email")
	if got := surface.disclosureCategory(); got != commsauthz.CategoryMarketing {
		t.Errorf("a legacy marketing send resolves disclosures under %q, want marketing: "+
			"it offers an unsubscribe surface, so it is advertising by this product's own "+
			"reckoning, and a pack's marketing-only obligations bind it", got)
	}
}

// TestAnExplicitCategoryIsNeverOverruled. The fallback exists for callers that
// said nothing, not to reclassify one that did.
func TestAnExplicitCategoryIsNeverOverruled(t *testing.T) {
	surface := surfaceFor(commsauthz.CategoryInvoiceOrPayment, "", "marketing_email")
	if got := surface.disclosureCategory(); got != commsauthz.CategoryInvoiceOrPayment {
		t.Errorf("an invoice resolved its disclosures under %q: a caller that named its "+
			"category is not reclassified by a legacy key beside it", got)
	}
}

func TestAMissingDisclosureBecomesARecordedFooterFinding(t *testing.T) {
	store := (&Store{}).WithDisclosures(&stubDisclosures{lines: []DisclosureLine{{Kind: "controller_identity"}}})
	rendered, err := store.deliverability(context.Background(), "As discussed.", "Re: your question",
		[]string{"anna@example.test"}, unsubscribeSurface{category: commsauthz.CategoryReplyToInbound}, ids.Nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.footerFindings) != 1 || rendered.footerFindings[0].Kind != "controller_identity" {
		t.Fatalf("missing disclosure findings = %v", rendered.footerFindings)
	}
	if rendered.transmitted != "As discussed." {
		t.Fatalf("body contains an invented disclosure: %q", rendered.transmitted)
	}
}

// TestAMetObligationStillSends guards the other direction: the refusal above
// must not fire on an installation that stated its particulars.
func TestAMetObligationStillSends(t *testing.T) {
	stub := &stubDisclosures{lines: []DisclosureLine{
		{Kind: "controller_identity", Text: "Beispiel GmbH\nHauptstr. 1, 10115 Berlin"},
	}}
	store := (&Store{}).WithDisclosures(stub)

	got, err := store.deliverability(context.Background(), "As discussed.", "Re: your question",
		[]string{"anna@example.test"},
		unsubscribeSurface{category: commsauthz.CategoryReplyToInbound}, ids.Nil, "")
	if err != nil {
		t.Fatalf("a fully stated installation was refused: %v", err)
	}
	if !strings.Contains(got.transmitted, "Beispiel GmbH") {
		t.Errorf("the disclosure is not in the body:\n%s", got.transmitted)
	}
}

// TestAVietnameseAdvertisingMailEndsWithTheAdvertiserThenTheRefusal. consent
// orders the lines, and the send keeps that order. The opt-out links follow
// the refusal directly, and nothing comes after them.
func TestAVietnameseAdvertisingMailEndsWithTheAdvertiserThenTheRefusal(t *testing.T) {
	advertiser := "Công ty Ví dụ\nĐiện thoại +84 28 3822 9999, email hello@example.com, trang https://example.com"
	refusal := "Quý khách có quyền từ chối nhận tất cả quảng cáo của Công ty Ví dụ. Cách từ chối:\n" +
		"gửi email đến privacy@example.com, gọi số +84 00 0000 0000 hoặc dùng trang https://example.com/stop"
	stub := &stubDisclosures{lines: []DisclosureLine{
		{Kind: "privacy_contact", Text: "privacy@example.com"},
		{Kind: "advertiser_contact", Text: advertiser},
		{Kind: "objection_route", Text: refusal},
	}}
	store := (&Store{}).WithDisclosures(stub).WithUnsubscribe(stubUnsubscribeLinker{token: "tok", ok: true})
	store.publicBaseURL = "https://mail.example.test"

	got, err := store.deliverability(context.Background(), "Ưu đãi tháng này dành cho quý khách.", "[QC] Ưu đãi",
		[]string{"an@example.vn"}, unsubscribeSurface{category: commsauthz.CategoryMarketing}, ids.Nil, "")
	if err != nil {
		t.Fatalf("deriving the message: %v", err)
	}
	body := got.transmitted
	at := strings.Index(body, advertiser)
	if at < 0 || !strings.HasPrefix(body[at+len(advertiser):], "\n\n"+refusal) {
		t.Fatalf("the advertiser block is not directly before the refusal section:\n%s", body)
	}
	rest := body[at+len(advertiser)+2+len(refusal):]
	if want := appendUnsubscribeFooter("", got.links, got.words); rest != want {
		t.Errorf("after the refusal comes %q, want only the opt-out links %q", rest, want)
	}
	if !strings.Contains(rest, "/#/unsubscribe/tok/") || !strings.Contains(rest, "/#/preferences/tok") {
		t.Errorf("the opt-out links %q do not carry the recipient's stop and preference links", rest)
	}
}

func TestFooterEvidenceBindsTheRedactedLinkDestination(t *testing.T) {
	first := footerEvidence(commsauthz.Evidence{}, sendDeliverability{}, "https://one.example.test/unsubscribe/token-redacted")
	next := footerEvidence(commsauthz.Evidence{}, sendDeliverability{}, "https://two.example.test/unsubscribe/token-redacted")
	if first.FooterDigest == next.FooterDigest {
		t.Fatal("a changed rendered footer kept the original acknowledgement scope")
	}
}
