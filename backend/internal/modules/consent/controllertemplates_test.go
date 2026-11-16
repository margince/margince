// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The installation's own wording, and the two properties that keep the lane
// from becoming a way around consent.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// exampleRefusal is the refusal the receipt template prints in these tests.
var exampleRefusal = RefusalTimes{
	ReceivedAt: time.Date(2026, time.March, 4, 2, 5, 0, 0, time.UTC),
	StoppedAt:  time.Date(2026, time.March, 4, 3, 30, 0, 0, time.UTC),
	UTCOffset:  7 * time.Hour,
}

// renderAnyTemplate renders key whichever renderer it takes, so a loop over
// the catalog reaches the receipt too.
func renderAnyTemplate(key string, expiresAt time.Time, language, purpose string) (Rendered, commsauthz.Category, error) {
	if controllerTemplates[key].refusalTimes {
		return renderReceipt(key, language, exampleRefusal)
	}
	return renderTemplateContext(key, expiresAt, language, purpose)
}

// TestEveryControllerTemplateCarriesExactlyOnePlaceholder holds the rendered
// body to the material it will be staged with.
//
// comms refuses a disagreement at staging, so a template with none would make
// its own send impossible and one with two would put the same live link in a
// message twice. Both are silent until somebody tries to send.
func TestEveryControllerTemplateCarriesExactlyOnePlaceholder(t *testing.T) {
	// EVERY language, because the body is assembled per language now. A
	// translation that dropped the placeholder, or repeated it, would make its
	// own send impossible on exactly the installations that speak it, while
	// English stayed green.
	for key := range controllerTemplates {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := renderAnyTemplate(key, time.Now().Add(72*time.Hour), string(language), "")
			if err != nil {
				t.Fatalf("rendering %q in %s: %v", key, language, err)
			}
			// THE COUNT MUST MATCH THE MATERIAL, which is comms' own rule
			// (checkPlaceholder): a template staged with a link carries exactly
			// one placeholder, and one staged with none carries zero. Demanding
			// one of every template would refuse the acknowledgement, which has
			// nothing to click by design — it goes to somebody who has just
			// asked the product to stop.
			want := 1
			if controllerTemplates[key].linkless {
				want = 0
			}
			if got := strings.Count(rendered.Body, linkPlaceholder); got != want {
				t.Errorf("template %q in %s carries %d link placeholder(s), want exactly %d: comms "+
					"refuses a body whose count disagrees with its material, so this template can "+
					"never be sent", key, language, got, want)
			}
			if RenderControllerPresentation(rendered, "Example Company", "") == "" || !strings.Contains(RenderControllerPresentation(rendered, "Example Company", ""), `lang="`+string(language)+`"`) {
				t.Errorf("template %s lacks localized markup", key)
			}
			if got := strings.Count(RenderControllerPresentation(rendered, "Example Company", ""), linkPlaceholder); got != want {
				t.Errorf("template %s markup has %d personal actions, want %d", key, got, want)
			}
			if rendered.Subject == "" {
				t.Errorf("template %q renders no subject line in %s", key, language)
			}
		}
	}
}

// TestEveryControllerTemplateResolvesToASubjectServingCategory is the property
// the lane's safety rests on.
//
// The send doors refuse any caller-claimed category where ServesTheSubject is
// true, precisely because those five pass a hard suppression. This lane is the
// sanctioned producer of them — so a template that resolved to an ORDINARY
// category would be the installation's own mail asking the engine the wrong
// question, and one that resolved to a category no validator evidences would be
// a message that can never be sent.
func TestEveryControllerTemplateResolvesToASubjectServingCategory(t *testing.T) {
	for key := range controllerTemplates {
		_, category, err := renderAnyTemplate(key, time.Time{}, string(mailcopy.Fallback), "")
		if err != nil {
			t.Fatalf("rendering %q: %v", key, err)
		}
		if !category.Valid() {
			t.Errorf("template %q resolves to %q, which is not in the category vocabulary", key, category)
			continue
		}
		if !category.ServesTheSubject() {
			t.Errorf("template %q resolves to %q, which does not serve the subject. The lane exists "+
				"to send the five categories a caller may not claim; a template resolving to an "+
				"ordinary one is the installation asking the engine the wrong question", key, category)
		}
		if !evidencedCategory(category) {
			t.Errorf("template %q resolves to %q, which no validator can evidence, so every "+
				"message from this template falls through to the legacy verdict and is denied",
				key, category)
		}
	}
}

// TestTheTemplatePlaceholderAgreesWithTheLane holds the two spellings of the
// placeholder together. consent may not import comms, so the value is repeated
// — and a repeated value is exactly the thing that drifts.
func TestTheTemplatePlaceholderAgreesWithTheLane(t *testing.T) {
	// The literal comms.LinkPlaceholder holds. Spelled out rather than imported
	// because the module boundary forbids the import; if comms changes its
	// value, this fails and names what to change.
	const commsSpelling = "{{confirmation-link}}"
	if linkPlaceholder != commsSpelling {
		t.Errorf("consent renders %q where comms substitutes %q — every controller message would "+
			"go out with the placeholder still in it, or be refused at staging for a count "+
			"mismatch", linkPlaceholder, commsSpelling)
	}
}

// TestAnUnregisteredTemplateIsRefused — the registry is what makes the lane
// closed. A body assembled at a call site must not reach it.
func TestAnUnregisteredTemplateIsRefused(t *testing.T) {
	if _, _, err := RenderControllerTemplate("marketing_blast", time.Time{}, string(mailcopy.Fallback)); err == nil {
		t.Fatal("an unregistered template rendered, so the installation's own words are not " +
			"fixed in code after all")
	}
	if ControllerTemplateRegistry().Registered("marketing_blast", 1) {
		t.Error("the registry recognises a template this build does not define")
	}
	if ControllerTemplateRegistry().Registered(TemplateRecordConfirmation, 99) {
		t.Error("the registry recognises a VERSION this build does not define, so wording could " +
			"change without the version that identifies it moving")
	}
}

// TestTheRenderedBodyNeverCarriesTheLink pins where the plaintext may be. The
// body is copied onto the delivery row, the timeline, the audit entry and the
// outbox event, so a link rendered into it lands in all four.
func TestTheRenderedBodyNeverCarriesTheLink(t *testing.T) {
	for key := range controllerTemplates {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := renderAnyTemplate(key, time.Now().Add(72*time.Hour), string(language), "")
			if err != nil {
				t.Fatalf("rendering %q in %s: %v", key, language, err)
			}
			if strings.Contains(rendered.Body, "http") {
				t.Errorf("template %q in %s renders a URL into its body:\n%s", key, language, rendered.Body)
			}
		}
	}
}

// The wording the installation sends in its own name cannot change quietly.
//
// A controller template is the only mail Margince writes as ITSELF rather than
// on a rep's behalf: the confirm-details link and the double-opt-in link. Both
// go to somebody who did not ask for them, and both are evidence — the consent
// proof records which version a contact was shown, so "what exactly did this
// contact read on 4 March" has to stay answerable after the wording moves on.
//
// That makes an unversioned edit a silent falsification rather than a typo fix.
// The proof row keeps pointing at version 1 while version 1's text no longer
// exists anywhere, and nothing in the tree notices.
//
// So each template's rendered text is pinned by hash. Editing a word fails this
// gate, and the fix is to bump the template's version alongside the wording,
// which is what keeps old proof rows honest about what they proved.
//
// It renders through the real path rather than hashing a hand-assembled copy of
// the subject and body, which would pin a second spelling and prove nothing
// about what an actual send carries.
//
// It sits beside the catalog rather than under gates/ because it compares one
// thing to itself: consent's wording against consent's own registry. gates/ is
// for holding two halves that live apart, and there is only one half here.

// pinnedWording is the hash of each template as it is registered today.
//
// The key carries the version AND the language, so a bump reads as a deliberate
// act and a translation cannot change quietly. Keyed by version alone, one
// language would hold the pin and the other two would be exempt — which is the
// half of this gate that matters most, since a German reader is the one who
// actually reads the German.
//
// It is a SNAPSHOT of what this build sends, not an archive of what it once
// sent. controllerTemplates is keyed by template key, so one version per key is
// registered at a time and a superseded row here would name wording nothing can
// render. The history that matters lives on consent_event, which stores the
// rendered policy_text with each proof — so an old version stays answerable
// from the proof row rather than from this map.
//
// gatekit:fixture the sha256 of each registered template's rendered wording
var pinnedWording = map[string]string{
	"record_check@1@de":           "a36d20c4968a63e2b0565807f522feb905049693328fa10010514eca8dde28d8",
	"record_check@1@en":           "0f6fbb22fe4c05c1516769b1a9d5171a7900ac9fa82a1b7d82f54ac93a8e2702",
	"record_check@1@vi":           "3f3e084561b8178ddabaa9676c65ba368c4797e24c0dcf2017975579b5a955f5",
	"consent_confirmation@7@de":   "945ebb0caa0cf98950c131cf04d704ccdfe12d5610e3e1436b09aaa4f1af2b12",
	"consent_confirmation@7@en":   "eeed2e8f83ce4228bff3b3ab05afd3b7c8cc67373e73d5cc90d5201a189bd1fe",
	"consent_confirmation@7@vi":   "52c3d3bc2d9e6b5a6a27518aae7a1598862b383a8178e6e7ca4ac3752a4a1fc0",
	"privacy_notice@5@de":         "afd5793238fb2ed046bafb2e7b39c42fedba0ef885f9e966e3609adf0b7e06f9",
	"privacy_notice@5@en":         "57940dbb8dbf58affde9b4f1fcf8026951c2112f2a64fd9fbab5f326534b1eec",
	"privacy_notice@5@vi":         "a5049a2fe4c29bb62dcc3a44f729d827dd3fd17318d25d3485d937ecd9b1062e",
	"restriction_lift@3@de":       "34752802be82b4a703cb52a4ca5d02eece3595ea71d955bb6ecebf0436fd0e0d",
	"restriction_lift@3@en":       "4e31b4aad01b43dce8700ef8abdbdbda4989e72138e7b9ad76a5bb3f3915e538",
	"restriction_lift@3@vi":       "3903eb642861db8d8a8f30329aff02a23d64577e4146435ce26dca84868bc257",
	"record_confirmation@5@de":    "1b43a49c716496407f12328ebcc68f43ee586909b6b8edfcc92e45212f58fb9f",
	"record_confirmation@5@en":    "5eea97facb021167689fd7d55a9f7eb111f6d5f3a97bce0d132c48a6146cffd7",
	"record_confirmation@5@vi":    "281e83f35490981fc497269357c73c08836ac37b610fa2752a1552e8cdf28f92",
	"optout_acknowledgement@4@de": "18f7fde93aba9471ace152b00294ab4366e309ebe8648ea15b0ade9ab5db0a7d",
	"optout_acknowledgement@4@en": "0e53dd1ec79223e43a8de554f0592f24e79970483a03adec516f9471404f7467",
	"optout_acknowledgement@4@vi": "f923d73de53266ad7d29874c8ca4ec3eb1118778984e07a86c5fc2730d49a63f",
	"request_receipt@1@de":        "b10035272f9adb8de0f9e23506722ff6e717dcaab8413b788bfcfc012b7caf7f",
	"request_receipt@1@en":        "ae9acd80f560a7d1409348434665e2409c23c39b7b2787bbaa7b05ac9b9b5f45",
	"request_receipt@1@vi":        "424c6d27ba1082a7a7057144b3e8699fdfe8da049d8b4c83b69feb97d908c154",
}

// TestEveryControllerTemplateIsPinnedToItsWording fails when a registered
// template's rendered text changes without its version changing with it.
func TestEveryControllerTemplateIsPinnedToItsWording(t *testing.T) {
	t.Parallel()

	// A fixed instant so the hash covers the WORDING and not the clock. The
	// rendered body carries an expiry date, and a real now() would make this
	// gate fail once a day for no reason anybody could act on.
	expires := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)

	keys := make([]string, 0, len(controllerTemplates))
	for key := range controllerTemplates {
		keys = append(keys, key)
	}
	// Under-recognition is the one way this must not fail: a registry that
	// returned nothing would loop zero times, report PASS, and leave every
	// template unpinned.
	if len(keys) < 2 {
		t.Fatalf("the catalog holds %d controller templates, want at least the "+
			"confirm-details and double-opt-in wordings: the gate has stopped seeing its subject",
			len(keys))
	}

	seen := map[string]bool{}
	for _, key := range keys {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := renderAnyTemplate(key, expires, string(language), "Example subscription")
			if err != nil {
				t.Errorf("rendering %s in %s: %v", key, language, err)
				continue
			}
			checkPin(t, seen, key, string(language), rendered)
		}
	}

	// A pin whose template is gone is a claim about wording nothing sends.
	for pin := range pinnedWording {
		if !seen[pin] {
			t.Errorf("%s is pinned here but no registered template renders it. Remove the row: "+
				"this map is what THIS build sends, and a pin nothing renders is a claim about "+
				"wording that no longer exists. What that contact was shown is recoverable from "+
				"consent_event.policy_text, not from here", pin)
		}
	}
}

// checkPin holds one rendered wording against its row, and records that the row
// was reached.
//
// Keyed key@version@locale. The wording is per language now, so a pin naming
// only key and version would hold whichever language happened to render last
// and silently exempt the other two — a German translation could then change
// without anything failing, on the installations that actually read it.
func checkPin(t *testing.T, seen map[string]bool, key, locale string, rendered Rendered) {
	t.Helper()

	sum := sha256.Sum256([]byte(rendered.Subject + "\x00" + rendered.Body))
	got := hex.EncodeToString(sum[:])
	pin := fmt.Sprintf("%s@%d@%s", key, rendered.Version, locale)
	seen[pin] = true

	want, pinned := pinnedWording[pin]
	switch {
	case !pinned:
		t.Errorf("%s is registered but not pinned here. If this is a NEW template or a new "+
			"language, add a row with %q. If you edited an existing one, bump its version and "+
			"move the pin with it — the version is what a consent proof names", pin, got)
	case want == "":
		t.Errorf("%s has an empty pin. Fill it with %q — an empty pin matches nothing "+
			"and silently exempts the wording it was meant to hold", pin, got)
	case got != want:
		t.Errorf("%s wording changed but its version did not.\n  pinned: %s\n  now:    %s\n\n"+
			"Bump the template's version and REPLACE this row with one for the new "+
			"version. Do not keep the old row: this map is what THIS build sends, and the "+
			"check below fails a pin nothing renders. What version %d actually showed a "+
			"contact is recoverable from consent_event.policy_text, which is where that "+
			"history belongs — not here",
			pin, want, got, rendered.Version)
	}
}

// TestTheExpiryRetainsItsTimeInEveryLanguage prevents a whole-day expiry promise.
func TestTheExpiryRetainsItsTimeInEveryLanguage(t *testing.T) {
	expires := time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	for key := range controllerTemplates {
		// A LINKLESS TEMPLATE HAS NO EXPIRY TO READ. The acknowledgement carries
		// nothing to click, so a date about when that nothing stops working
		// would be a sentence about a link the reader does not have.
		if controllerTemplates[key].linkless {
			continue
		}
		for _, language := range mailcopy.Languages() {
			rendered, _, err := RenderControllerTemplate(key, expires, string(language))
			if err != nil {
				t.Fatalf("rendering %q in %s: %v", key, language, err)
			}
			if !strings.Contains(rendered.Body, mailcopy.For(string(language)).Instant(expires)) {
				t.Errorf("template %q in %s does not carry the exact localized expiry:\n%s",
					key, language, rendered.Body)
			}
			// The month name is what an English layout leaks. Asserting the ISO
			// date alone would pass on a body carrying both.
			if strings.Contains(rendered.Body, "March") {
				t.Errorf("template %q in %s spells the month in English:\n%s",
					key, language, rendered.Body)
			}
		}
	}
}

// TestTheExpiryLineTakesExactlyOneDate holds the one format string in a
// controller mail.
//
// ConfirmExpiry is the only catalog line with a verb in it, and Fprintf answers
// a mismatch with text rather than an error: two verbs render
// "%!s(MISSING)" and a stray percent renders "%!(NOVERB)", both of which reach
// the mailbox as-is. The pin would change, but a translator repinning their own
// change would read "wording changed" and repin it — so the wrong thing has to
// be named here.
func TestTheExpiryLineTakesExactlyOneDate(t *testing.T) {
	for _, language := range mailcopy.Languages() {
		line := mailcopy.For(string(language)).ConfirmExpiry
		if got := strings.Count(line, "%s"); got != 1 {
			t.Errorf("the %s expiry line carries %d %%s verbs, want exactly 1: %q",
				language, got, line)
		}
		// Every percent in the line must be that one verb. A stray one — an
		// escaped literal, a %d left by a copy — renders as a diagnostic.
		if got := strings.Count(line, "%"); got != 1 {
			t.Errorf("the %s expiry line carries %d percent signs and only one verb is allowed: %q",
				language, got, line)
		}
	}
}

// evidencedCategory reports whether some validator can support this category.
//
// TWO SHAPES OF EVIDENCE, which is why this is not confirmKindFor. The three
// link-carrying categories are evidenced by a live confirm_token, and
// confirmKindFor names which. The opt-out acknowledgement carries nothing to
// click and is evidenced by the standing stop it acknowledges, so asking only
// the first question would call a sendable template unsendable.
//
// It mirrors decideResolved's dispatch (authorizevalidators.go) and must move
// with it: a category that gains a validator there and not here reads as
// unsendable, and one that loses its arm there reads as fine until a message
// from it is denied.
func evidencedCategory(category commsauthz.Category) bool {
	if _, linked := confirmKindFor(category); linked {
		return true
	}
	return category == commsauthz.CategoryOptoutConfirmation
}

func TestQueuedControllerVersionsRemainRecognized(t *testing.T) {
	registry := ControllerTemplateRegistry()
	for key, old := range map[string]int{TemplateRecordConfirmation: 2, TemplateConsentConfirmation: 3, TemplatePrivacyNotice: 1, TemplateOptOutAcknowledgement: 1} {
		current, ok := ControllerTemplateVersion(key)
		if !ok || current <= old || !registry.Registered(key, old) || !registry.Registered(key, current) || registry.Registered(key, 0) || registry.Registered(key, current+1) {
			t.Errorf("invalid current/queued version policy for %s", key)
		}
	}
}

func TestOnlyTheSubscriptionTemplateNamesASubscription(t *testing.T) {
	for _, key := range []string{TemplateRecordConfirmation, TemplateConsentConfirmation, TemplatePrivacyNotice} {
		rendered, _, err := renderTemplateContext(key, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), "de", "Produktneuigkeiten")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(rendered.Body, "Thema: Produktneuigkeiten"); got != (key == TemplateConsentConfirmation) {
			t.Fatalf("subscription and correction contexts crossed: %s", rendered.Body)
		}
	}
}

// TestTheReceiptPrintsBothTimesInVietnamTime holds Decree 91/2020/ND-CP Art.
// 20.4(a) and the wording the Vietnamese receipt is required to carry.
func TestTheReceiptPrintsBothTimesInVietnamTime(t *testing.T) {
	rendered, _, err := RenderOptOutAcknowledgement("vi", exampleRefusal)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Xác nhận đã nhận yêu cầu từ chối nhận thư điện tử quảng cáo"; rendered.Subject != want {
		t.Errorf("subject = %q, want %q", rendered.Subject, want)
	}
	want := "Chúng tôi đã nhận được yêu cầu từ chối nhận thư điện tử quảng cáo của quý khách lúc 09:05, 04/03/2026 (giờ Việt Nam). " +
		"Chúng tôi đã ngừng gửi thư điện tử quảng cáo cho các mục đích quảng cáo quý khách đã từ chối kể từ 10:30, 04/03/2026 (giờ Việt Nam). " +
		"Thư đã gửi trước thời điểm này vẫn có thể đến hộp thư của quý khách.\n\n" +
		"Quý khách không cần trả lời hay thực hiện thêm thao tác nào.\n"
	if rendered.Body != want {
		t.Errorf("body =\n%q\nwant\n%q", rendered.Body, want)
	}
	if _, _, err := RenderOptOutAcknowledgement("vi", RefusalTimes{StoppedAt: exampleRefusal.StoppedAt}); err == nil {
		t.Error("a receipt rendered with no received time; Art. 20.4(a) requires it")
	}
}

// TestOnlyTheReceiptDropsThePoweredByLine. Art. 20.4(b) bars advertising in the
// receipt; every other controller mail keeps the attribution.
func TestOnlyTheReceiptDropsThePoweredByLine(t *testing.T) {
	for key := range controllerTemplates {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := renderAnyTemplate(key, exampleRefusal.StoppedAt.Add(72*time.Hour), string(language), "")
			if err != nil {
				t.Fatal(err)
			}
			markup := RenderControllerPresentation(rendered, "Example Company", "")
			attributed := strings.Contains(markup, mailcopy.For(string(language)).PoweredBy)
			receipt := controllerTemplates[key].unbranded
			if receipt && (attributed || strings.Contains(markup+rendered.Subject+rendered.Body, "Margince")) {
				t.Errorf("the %s %s names Margince:\n%s", language, key, markup)
			}
			if !receipt && !attributed {
				t.Errorf("%s in %s lost its attribution line", key, language)
			}
		}
	}
}

// TestOnlyTheVietnameseReceiptNamesVietnam. The shared wording prints the
// offset the rules declare; the Vietnamese one carries the decree's own words.
func TestOnlyTheVietnameseReceiptNamesVietnam(t *testing.T) {
	for _, language := range mailcopy.Languages() {
		rendered, _, err := RenderOptOutAcknowledgement(string(language), exampleRefusal)
		if err != nil {
			t.Fatal(err)
		}
		names := strings.Contains(strings.ToLower(rendered.Body), "vietnam") || strings.Contains(rendered.Body, "Việt Nam")
		if language == "vi" {
			if !strings.Contains(rendered.Body, "(giờ Việt Nam)") {
				t.Errorf("the Vietnamese receipt lost the decree's words:\n%s", rendered.Body)
			}
			continue
		}
		if names || strings.Count(rendered.Body, "(UTC+07:00)") != 2 {
			t.Errorf("the %s receipt names a country or misses the offset:\n%s", language, rendered.Body)
		}
	}
}
