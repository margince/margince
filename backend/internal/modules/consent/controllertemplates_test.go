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
)

// TestEveryControllerTemplateCarriesExactlyOnePlaceholder holds the rendered
// body to the material it will be staged with.
//
// comms refuses a disagreement at staging, so a template with none would make
// its own send impossible and one with two would put the same live link in a
// message twice. Both are silent until somebody tries to send.
func TestEveryControllerTemplateCarriesExactlyOnePlaceholder(t *testing.T) {
	for key := range controllerTemplates {
		rendered, _, err := RenderControllerTemplate(key, time.Now().Add(72*time.Hour))
		if err != nil {
			t.Fatalf("rendering %q: %v", key, err)
		}
		if got := strings.Count(rendered.Body, linkPlaceholder); got != 1 {
			t.Errorf("template %q carries %d link placeholder(s), want exactly 1: comms refuses "+
				"a body whose count disagrees with its material, so this template can never be sent",
				key, got)
		}
		if rendered.Subject == "" {
			t.Errorf("template %q renders no subject line", key)
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
		_, category, err := RenderControllerTemplate(key, time.Time{})
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
	if _, _, err := RenderControllerTemplate("marketing_blast", time.Time{}); err == nil {
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
		rendered, _, err := RenderControllerTemplate(key, time.Now().Add(72*time.Hour))
		if err != nil {
			t.Fatalf("rendering %q: %v", key, err)
		}
		if strings.Contains(rendered.Body, "http") {
			t.Errorf("template %q renders a URL into its body:\n%s", key, rendered.Body)
		}
	}
}

// The wording the installation sends in its own name cannot change quietly.
//
// A controller template is the only mail Margince writes as ITSELF rather than
// on a rep's behalf: the confirm-details link and the double-opt-in link. Both
// go to somebody who did not ask for them, and both are evidence — the consent
// proof records which version a person was shown, so "what exactly did this
// person read on 4 March" has to stay answerable after the wording moves on.
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
// The key carries the version so a bump reads as a deliberate act: changing the
// words alone fails, and the fix is to move the version and the pin together.
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
	"record_confirmation@1":  "d53b469155a1219102d50008f40fa992447915a7354fbc51755c39bb9b9aec16",
	"consent_confirmation@1": "65bf988d3efe84768d89cace0cd7bbab93b433269b592fe6bebaa11587be8320",
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
		rendered, _, err := RenderControllerTemplate(key, expires)
		if err != nil {
			t.Errorf("rendering %s: %v", key, err)
			continue
		}

		sum := sha256.Sum256([]byte(rendered.Subject + "\x00" + rendered.Body))
		got := hex.EncodeToString(sum[:])
		pin := fmt.Sprintf("%s@%d", key, rendered.Version)
		seen[pin] = true

		want, pinned := pinnedWording[pin]
		if !pinned {
			t.Errorf("%s is registered but not pinned here. If this is a NEW template, "+
				"add a row with %q. If you edited an existing one, bump its version and move "+
				"the pin with it — the version is what a consent proof names",
				pin, got)
			continue
		}
		if want == "" {
			t.Errorf("%s has an empty pin. Fill it with %q — an empty pin matches nothing "+
				"and silently exempts the wording it was meant to hold", pin, got)
			continue
		}
		if got != want {
			t.Errorf("%s wording changed but its version did not.\n  pinned: %s\n  now:    %s\n\n"+
				"Bump the template's version and add a NEW row here, keeping the old one. "+
				"consent_event records which version a person was shown, so editing version %d "+
				"in place makes every existing proof row describe text that no longer exists",
				pin, want, got, rendered.Version)
		}
	}

	// A pin whose template is gone is a claim about wording nothing sends.
	for pin := range pinnedWording {
		if !seen[pin] {
			t.Errorf("%s is pinned here but no registered template renders it. Remove the row: "+
				"this map is what THIS build sends, and a pin nothing renders is a claim about "+
				"wording that no longer exists. What that person was shown is recoverable from "+
				"consent_event.policy_text, not from here", pin)
		}
	}
}
