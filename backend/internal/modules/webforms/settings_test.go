// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// refusedField is the field a refusal names, or "" when the input was taken.
func refusedField(err error) string {
	if bad, ok := errors.AsType[*BadInputError](err); ok {
		return bad.Field
	}
	return ""
}

func TestANewFormIsRefusedOnTheFieldItGotWrong(t *testing.T) {
	site := []string{"https://www.example.com"}
	secret := ptr("a-builder-secret-of-length")
	for name, tc := range map[string]struct {
		in    CreateInput
		field string
	}{
		"no name":                     {CreateInput{Sender: SenderBrowser, AllowedOrigins: site}, "name"},
		"unknown sender":              {CreateInput{Name: "F", Sender: "fax"}, "sender"},
		"browser without a website":   {CreateInput{Name: "F", Sender: SenderBrowser}, "allowed_origins"},
		"server with a website":       {CreateInput{Name: "F", Sender: SenderServer, AllowedOrigins: site}, "allowed_origins"},
		"builder without its secret":  {CreateInput{Name: "F", Sender: SenderTally}, "sender_secret"},
		"server with a pasted secret": {CreateInput{Name: "F", Sender: SenderServer, SenderSecret: secret}, "sender_secret"},
		"consent without wording":     {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, ConsentRequired: true}, colConsentWording},
		"plain http redirect":         {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, RedirectURL: ptr("http://example.com/thanks")}, colRedirectURL},
		"map onto no lead field":      {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, FieldMap: map[string]string{"x": "salary"}}, "field_map"},
		"a builder form":              {CreateInput{Name: "F", Sender: SenderTally, SenderSecret: secret}, ""},
	} {
		if got := refusedField(tc.in.normalize()); got != tc.field {
			t.Errorf("%s: refused field = %q, want %q", name, got, tc.field)
		}
	}
}

func TestANewFormLandsLeadsAsWebsiteLeadsUnlessToldOtherwise(t *testing.T) {
	in := CreateInput{Name: " Contact ", Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com/x"}}
	if err := in.normalize(); err != nil {
		t.Fatal(err)
	}
	if in.Name != "Contact" || in.LeadOrigin == nil || *in.LeadOrigin != defaultLeadOrigin {
		t.Fatalf("normalized = %+v", in)
	}
}

func TestAnUpdateIsCheckedAgainstTheFormItLeaves(t *testing.T) {
	browser := Form{
		Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com"},
		RedirectURL: ptr("https://www.example.com/thanks"), ConsentWording: ptr("You may answer me."),
	}
	server := Form{Sender: SenderServer}
	for name, tc := range map[string]struct {
		form  Form
		in    UpdateInput
		field string
	}{
		"blank name":                   {browser, UpdateInput{Name: ptr(" ")}, "name"},
		"every website removed":        {browser, UpdateInput{AllowedOrigins: &[]string{}}, "allowed_origins"},
		"a website on a server form":   {server, UpdateInput{AllowedOrigins: &[]string{"https://www.example.com"}}, "allowed_origins"},
		"a pasted secret on a server":  {server, UpdateInput{SenderSecret: ptr("a-builder-secret-of-length")}, "sender_secret"},
		"a state that is not one":      {browser, UpdateInput{State: ptr("deleted")}, "state"},
		"consent once wording cleared": {browser, UpdateInput{ConsentRequired: ptr(true), Clear: []string{colConsentWording}}, colConsentWording},
		"a pause":                      {browser, UpdateInput{State: ptr(StatePaused)}, ""},
	} {
		_, err := tc.in.merged(tc.form)
		if got := refusedField(err); got != tc.field {
			t.Errorf("%s: refused field = %q, want %q", name, got, tc.field)
		}
	}
}

func TestAClearedSettingIsGoneFromTheMergedForm(t *testing.T) {
	form := Form{
		Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com"},
		LeadOrigin: ptr("website"), RedirectURL: ptr("https://www.example.com/thanks"),
		SuccessMessage: ptr("Thanks"), ConsentPolicyVersion: ptr("v1"),
	}
	in := UpdateInput{
		Name: ptr("Renamed"), SuccessMessage: ptr("Danke"),
		Clear: []string{colLeadOrigin, colOwnerID, colRedirectURL, colConsentPolicyVersion, colConsentWording, colSuccessMessage},
	}
	got, err := in.merged(form)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.LeadOrigin != nil || got.RedirectURL != nil ||
		got.ConsentPolicyVersion != nil || got.SuccessMessage != nil {
		t.Fatalf("merged = %+v, want every cleared setting empty", got)
	}
}
