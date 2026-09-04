// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The installation's own wording, and the two properties that keep the lane
// from becoming a way around consent.

import (
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
