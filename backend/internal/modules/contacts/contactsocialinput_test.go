// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/socialplatform"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// SocialLinkedIn is spelled as a literal so the gate censuses can read it; it
// names the same key the platform list does.
func TestTheLinkedInSlotIsTheListedLinkedInKey(t *testing.T) {
	t.Parallel()
	if SocialLinkedIn != socialplatform.LinkedIn {
		t.Fatalf("SocialLinkedIn = %q, the platform list says %q", SocialLinkedIn, socialplatform.LinkedIn)
	}
}

func TestASocialKeyOutsideTheListIsRefusedByName(t *testing.T) {
	t.Parallel()
	_, err := contactSocialInputs(map[string]any{"linkedin_url": "https://www.linkedin.com/in/someone"})
	var refusal *values.ParseError
	if !errors.As(err, &refusal) || refusal.Code != "social_platform_unknown" || refusal.Field != "social.linkedin_url" {
		t.Fatalf("an unknown social key answered %v, want social_platform_unknown on social.linkedin_url", err)
	}
}

func TestTheLinkedInKeyTakesOnlyAProfileLink(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"https://example.com/x", "jane"} {
		_, err := contactSocialInputs(map[string]any{SocialLinkedIn: raw})
		var refusal *values.ParseError
		if !errors.As(err, &refusal) || refusal.Code != "linkedin_url_not_linkedin" {
			t.Errorf("social.linkedin %q answered %v, want linkedin_url_not_linkedin", raw, err)
		}
	}
	rows, err := contactSocialInputs(map[string]any{SocialLinkedIn: "http://LinkedIn.com/in/Jane/", "github": "jane", "twitter": ""})
	if err != nil {
		t.Fatal(err)
	}
	want := []ContactSocialInput{
		{Platform: "github", Handle: "jane"},
		{Platform: SocialLinkedIn, Handle: "https://www.linkedin.com/in/Jane"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %+v, want %+v: the profile normalised and the empty twitter left as a removal", rows, want)
	}
}

func TestLinkedInURLWritesTheSocialLinkedInKey(t *testing.T) {
	t.Parallel()
	link := "https://www.linkedin.com/in/ada"
	folded, err := socialWithLinkedInURL(map[string]any{"github": "ada"}, &link, false)
	if err != nil || !reflect.DeepEqual(folded, map[string]any{"github": "ada", SocialLinkedIn: link}) {
		t.Fatalf("folded = %v, %v; want linkedin_url under social.linkedin beside github", folded, err)
	}
	// Both spellings naming one profile is no conflict.
	if _, err := socialWithLinkedInURL(map[string]any{SocialLinkedIn: "http://linkedin.com/in/ada/"}, &link, false); err != nil {
		t.Errorf("one profile under both spellings answered %v", err)
	}
	cleared, err := socialWithLinkedInURL(nil, nil, true)
	if err != nil || !reflect.DeepEqual(cleared, map[string]any{SocialLinkedIn: ""}) {
		t.Errorf("a cleared linkedin_url folded to %v, %v; want the removal of social.linkedin", cleared, err)
	}
	if untouched, err := socialWithLinkedInURL(nil, nil, false); err != nil || untouched != nil {
		t.Errorf("no linkedin_url folded to %v, %v; want the social map left alone", untouched, err)
	}
}

func TestTwoDifferentProfilesUnderTheTwoSpellingsAreRefused(t *testing.T) {
	t.Parallel()
	link := "https://www.linkedin.com/in/ada"
	_, err := socialWithLinkedInURL(map[string]any{SocialLinkedIn: "https://www.linkedin.com/in/someone-else"}, &link, false)
	var refusal *values.ParseError
	if !errors.As(err, &refusal) || refusal.Code != "linkedin_url_conflicts_with_social" ||
		!strings.Contains(refusal.Message, "linkedin_url") || !strings.Contains(refusal.Message, "social.linkedin") {
		t.Fatalf("two profiles answered %v, want linkedin_url_conflicts_with_social naming both fields", err)
	}
}
