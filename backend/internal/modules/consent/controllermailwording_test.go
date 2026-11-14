// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// TestTheLinkExpiryIsPrintedInTheZoneItIsGivenIn. renderConfirmMail hands the
// expiry over in the installation's zone, so the mail prints that local time and
// names its offset.
func TestTheLinkExpiryIsPrintedInTheZoneItIsGivenIn(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2026, 11, 28, 11, 0, 0, 0, time.UTC).In(berlin)
	rendered, _, err := RenderControllerTemplate(TemplateRecordConfirmation, expires, "de")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Body, "28.11.2026, 12:00 Uhr (UTC+01:00)") {
		t.Errorf("the expiry is not printed in Berlin time:\n%s", rendered.Body)
	}
}

// TestEveryDisclosedParticularIsLabelledInTheMailsLanguage. The particulars are
// typed in one language; the label over each is the mail's.
func TestEveryDisclosedParticularIsLabelledInTheMailsLanguage(t *testing.T) {
	kinds := []messagingrules.DisclosureKind{
		messagingrules.ControllerIdentity, messagingrules.PrivacyContact,
		messagingrules.ObjectionRoute, messagingrules.AdvertiserContact,
	}
	for _, language := range mailcopy.Languages() {
		words := mailcopy.For(string(language))
		for _, kind := range kinds {
			got := labelledDisclosure(DisclosureLine{Kind: string(kind), Text: "Example Trading GmbH\n"}, words)
			label, text, found := strings.Cut(got, "\n")
			if !found || label == "" || text != "Example Trading GmbH" {
				t.Errorf("%s/%s: disclosure printed as %q, want a label line over the particular", language, kind, got)
			}
		}
	}
	if got := mailcopy.For("de").DisclosureController; got != "Verantwortlicher:" {
		t.Errorf("the German controller label is %q", got)
	}
}
