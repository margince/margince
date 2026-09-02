// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The three destinations one send derives, and the language they speak.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The header keeps the machine endpoint. A mailbox provider POSTs it
// without a browser, and moving it would break RFC 8058 one-click.
func TestTheHeaderKeepsTheOneClickAPIURL(t *testing.T) {
	links := unsubscribeLinksFor("https://crm.example.com", "tok", "marketing_email", textlang.English)
	want := "https://crm.example.com/v1/public/preferences/tok/unsubscribe?purpose=marketing_email"
	if links.oneClick != want {
		t.Errorf("oneClick = %q, want %q", links.oneClick, want)
	}
	if got := listUnsubscribeHeader(links.oneClick); got != "<"+want+">" {
		t.Errorf("header = %q, want the bracketed one-click URL", got)
	}
}

// The two VISIBLE links are pages. This is the defect the whole change is
// about: a person clicking the footer used to reach a POST-only endpoint
// (405) and a JSON document.
func TestTheVisibleLinksArePagesNotTheAPI(t *testing.T) {
	links := unsubscribeLinksFor("https://crm.example.com", "tok", "business_correspondence", textlang.German)
	for name, got := range map[string]string{"unsubscribe": links.unsubscribe, "manage": links.manage} {
		if strings.Contains(got, "/v1/") {
			t.Errorf("%s = %q — a visible link must not point at the API", name, got)
		}
		if !strings.Contains(got, "/#/") {
			t.Errorf("%s = %q, want a hash route", name, got)
		}
		if !strings.HasSuffix(got, "?lang=de") {
			t.Errorf("%s = %q, want the message's language carried", name, got)
		}
	}
	if !strings.Contains(links.unsubscribe, "/#/unsubscribe/tok/business_correspondence") {
		t.Errorf("unsubscribe = %q, want token and purpose in the path", links.unsubscribe)
	}
	if !strings.Contains(links.manage, "/#/preferences/tok") {
		t.Errorf("manage = %q, want the preference centre", links.manage)
	}
}

// Plain and markup are two renderings of ONE message, so they must offer
// the same capability. A recipient's ability to unsubscribe must not
// depend on which alternative their mail client chose to show.
func TestBothFootersCarryOneTokenPurposeAndLanguage(t *testing.T) {
	links := unsubscribeLinksFor("https://crm.example.com", "tok", "newsletter", textlang.German)
	words := mailcopy.For("de")
	plain := appendUnsubscribeFooter("Guten Tag", links, words)
	markup := sendDeliverability{links: links, words: words}.htmlFooter()

	for _, want := range []string{links.unsubscribe, links.manage} {
		if !strings.Contains(plain, want) {
			t.Errorf("the plain footer is missing %q", want)
		}
		if !strings.Contains(markup, want) {
			t.Errorf("the markup footer is missing %q", want)
		}
	}
	if !strings.Contains(plain, words.UnsubscribeLabel) {
		t.Errorf("the plain footer does not speak the message's language: %q", plain)
	}
}

// The recorded copy keeps the footer's SHAPE and loses only the
// capability: a reader of the timeline sees that the send carried a
// working link and which purpose it pointed at, and cannot use it.
func TestTheRecordedFooterCarriesOnlyTheRedactedToken(t *testing.T) {
	const live = "pref_secret_value"
	redacted := unsubscribeLinksFor("https://crm.example.com", redactedToken, "newsletter", textlang.English)
	recorded := appendUnsubscribeFooter("Body", redacted, mailcopy.For("en"))

	if strings.Contains(recorded, live) {
		t.Errorf("the recorded body carries the live token: %q", recorded)
	}
	if !strings.Contains(recorded, redactedToken) {
		t.Errorf("the recorded body lost the footer's shape: %q", recorded)
	}
	if !strings.Contains(recorded, "/#/unsubscribe/"+redactedToken+"/newsletter") {
		t.Errorf("the recorded body does not name the purpose: %q", recorded)
	}
}

// A purpose or token carrying a separator must not be able to reshape the
// URL it lands in.
func TestTokenAndPurposeAreEscaped(t *testing.T) {
	links := unsubscribeLinksFor("https://crm.example.com", "tok/../evil", "news letter/x", textlang.English)
	for name, got := range map[string]string{"unsubscribe": links.unsubscribe, "oneClick": links.oneClick} {
		if strings.Contains(got, "/../") {
			t.Errorf("%s = %q — a token reshaped the path", name, got)
		}
		if strings.Contains(got, "letter/x") {
			t.Errorf("%s = %q — a purpose reshaped the path", name, got)
		}
	}
}

// A German message gets a German footer. The English footer under German
// prose is the product speaking two languages in one message.
func TestAGermanBodyGetsAGermanFooter(t *testing.T) {
	store := &Store{}
	const body = "Guten Tag, ich melde mich noch einmal wegen des Angebots und der offenen Fragen dazu."
	if got := store.footerLanguage(context.Background(), body, "Angebot"); got != textlang.German {
		t.Errorf("footerLanguage = %q, want de", got)
	}
}

// A body too short to read falls through to the subject, then to the
// installation's own language. textlang.Detect is deliberately biased to
// Unknown, so this tier is reached often rather than rarely.
func TestAShortBodyFallsThroughTheLadder(t *testing.T) {
	store := (&Store{}).WithBaseLanguage(BaseLanguageFunc(func(context.Context) string { return "vi" }))
	if got := store.footerLanguage(context.Background(), "Danke!", ""); got != textlang.Vietnamese {
		t.Errorf("footerLanguage = %q, want the installation's vi", got)
	}
	const subject = "Ihre Anfrage zu unserem Angebot und den offenen Punkten"
	if got := store.footerLanguage(context.Background(), "Danke!", subject); got != textlang.German {
		t.Errorf("footerLanguage = %q, want de from the subject", got)
	}
}

// No reader wired, nothing detectable: English rather than a guess.
func TestAnUnreadableMessageWithNoInstallationLanguageIsEnglish(t *testing.T) {
	store := &Store{}
	if got := store.footerLanguage(context.Background(), "ok", ""); got != textlang.English {
		t.Errorf("footerLanguage = %q, want en", got)
	}
}

// The send refuses an origin the recipient could not open. This is the
// configuration that produced the incident.
func TestATokenizedSendRefusesAnUnreachableOrigin(t *testing.T) {
	store := (&Store{}).WithPublicBaseURL("http://localhost:8080")
	err := store.publicOriginUsable()
	if err == nil {
		t.Fatal("a localhost origin was admitted for a tokenized send")
	}
	var fault *PublicOriginUnusableError
	if !asPublicOriginFault(err, &fault) {
		t.Fatalf("error = %T, want a PublicOriginUnusableError the caller can act on", err)
	}
	code, message := fault.MessageFault()
	if code != "public_origin_unusable" {
		t.Errorf("code = %q", code)
	}
	if strings.Contains(message, "localhost:8080") {
		t.Errorf("the refusal echoed the configured origin: %q", message)
	}
}

// An unconfigured origin keeps the sentence the send path has always used.
func TestAnUnconfiguredOriginStillSaysSo(t *testing.T) {
	err := (&Store{}).publicOriginUsable()
	if err == nil || !strings.Contains(err.Error(), "public base URL is not configured") {
		t.Errorf("error = %v, want the long-standing wording", err)
	}
}

func asPublicOriginFault(err error, target **PublicOriginUnusableError) bool {
	fault, ok := err.(*PublicOriginUnusableError)
	if ok {
		*target = fault
	}
	return ok
}
