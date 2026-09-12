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
)

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
			rendered, _, err := RenderControllerTemplate(key, time.Now().Add(72*time.Hour), string(language))
			if err != nil {
				t.Fatalf("rendering %q in %s: %v", key, language, err)
			}
			if got := strings.Count(rendered.Body, linkPlaceholder); got != 1 {
				t.Errorf("template %q in %s carries %d link placeholder(s), want exactly 1: comms "+
					"refuses a body whose count disagrees with its material, so this template can "+
					"never be sent", key, language, got)
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
		_, category, err := RenderControllerTemplate(key, time.Time{}, string(mailcopy.Fallback))
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
		if _, evidenced := confirmKindFor(category); !evidenced {
			t.Errorf("template %q resolves to %q, which validateConfirmation cannot evidence, so "+
				"every message from this template falls through to the legacy verdict and is denied",
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
			rendered, _, err := RenderControllerTemplate(key, time.Now().Add(72*time.Hour), string(language))
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
	"privacy_notice@1@de":       "f22fd4fc7fe4e266bd6f19566cf1c168d9981106ee8bfe64d7d43758b1bd8b3a",
	"privacy_notice@1@en":       "9bcfa68cac9cece7974e57c71472434f4baf865a4a0c8e12970fe0b3f1212dd4",
	"privacy_notice@1@vi":       "5fc479c528b31e79df080e9f07cbfedd15e6e67373ed73776b29b7513da9bef2",
	"record_confirmation@2@en":  "ae6261f551d0f39945b720db03b9ef2f51a36516b88eeff0b1afae80808e0c28",
	"record_confirmation@2@de":  "3ba76e0c75f2dd619ad4666d3452607b87a638ea1183e3188ace7bd7d4ac6b06",
	"record_confirmation@2@vi":  "28894b02130d640779a9fd4550a2f987a4925c04aaf2749679d44708eae9d2a7",
	"consent_confirmation@2@en": "05485c736c4971864938a0a4100a69b0533b7f9792e1d75820299461c043233e",
	"consent_confirmation@2@de": "78ed420da7fa53e0cbfc5f02996520e27f8ba0fbb84b1c435b52abbe0da30054",
	"consent_confirmation@2@vi": "e224aa3f581bf8b2fccb3468364abe52f0f6467e3dd045c9e2b056a96fd4b7b3",
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
			rendered, _, err := RenderControllerTemplate(key, expires, string(language))
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

// TestTheExpiryDateReadsTheSameInEveryLanguage is the one part of a rendered
// message that is not copy, and it was wrong first time round.
//
// Go's reference layouts name the month in English, so "2 January 2006" put
// "9 March 2026" inside a German sentence — the single visible place where a
// catalog of translations would still have shipped English. An ISO date carries
// no month name to get wrong.
func TestTheExpiryDateReadsTheSameInEveryLanguage(t *testing.T) {
	expires := time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	for key := range controllerTemplates {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := RenderControllerTemplate(key, expires, string(language))
			if err != nil {
				t.Fatalf("rendering %q in %s: %v", key, language, err)
			}
			if !strings.Contains(rendered.Body, "2026-03-09") {
				t.Errorf("template %q in %s does not carry the ISO expiry date:\n%s",
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
