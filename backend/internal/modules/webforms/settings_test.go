// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"strings"
	"testing"
)

// refusedField is the field a refusal names, or "" when the input was taken.
func refusedField(err error) string {
	if bad, ok := errors.AsType[*BadInputError](err); ok {
		return bad.Field
	}
	return ""
}

func TestANewFormIsRefusedOnTheFieldItGotWrong(t *testing.T) {
	site := []string{"https://www.example.com"}
	secret := new("a-builder-secret-of-length")
	for name, tc := range map[string]struct {
		in    CreateInput
		field string
	}{
		"no name":                     {CreateInput{Sender: SenderBrowser, AllowedOrigins: site}, "name"},
		"unknown sender":              {CreateInput{Name: "F", Sender: "fax"}, "sender"},
		"browser without a website":   {CreateInput{Name: "F", Sender: SenderBrowser}, "allowed_origins"},
		"server with a website":       {CreateInput{Name: "F", Sender: SenderServer, AllowedOrigins: site}, "allowed_origins"},
		"webflow before its secret":   {CreateInput{Name: "F", Sender: SenderWebflow}, ""},
		"tally without a secret":      {CreateInput{Name: "F", Sender: SenderTally}, ""},
		"server with a pasted secret": {CreateInput{Name: "F", Sender: SenderServer, SenderSecret: secret}, "sender_secret"},
		"consent without wording":     {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, ConsentRequired: true}, colConsentWording},
		"plain http redirect":         {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, RedirectURL: new("http://example.com/thanks")}, colRedirectURL},
		"map onto no lead field":      {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, FieldMap: map[string]string{"x": "salary"}}, "field_map"},
		"a policy link on plain http": {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, PrivacyNoticeURL: new("http://example.com/privacy")}, colPrivacyNoticeURL},
		"a notice past its bound":     {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, PrivacyNotice: new(strings.Repeat("a", maxPrivacyNotice+1))}, colPrivacyNotice},
		"a notice and its policy":     {CreateInput{Name: "F", Sender: SenderBrowser, AllowedOrigins: site, PrivacyNotice: new("We answer your request."), PrivacyNoticeURL: new("https://www.example.com/privacy")}, ""},
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
		RedirectURL: new("https://www.example.com/thanks"), ConsentWording: new("You may answer me."),
	}
	server := Form{Sender: SenderServer}
	for name, tc := range map[string]struct {
		form  Form
		in    UpdateInput
		field string
	}{
		"blank name":                   {browser, UpdateInput{Name: new(" ")}, "name"},
		"every website removed":        {browser, UpdateInput{AllowedOrigins: &[]string{}}, "allowed_origins"},
		"a website on a server form":   {server, UpdateInput{AllowedOrigins: &[]string{"https://www.example.com"}}, "allowed_origins"},
		"a pasted secret on a server":  {server, UpdateInput{SenderSecret: new("a-builder-secret-of-length")}, "sender_secret"},
		"a state that is not one":      {browser, UpdateInput{State: new("deleted")}, "state"},
		"consent once wording cleared": {browser, UpdateInput{ConsentRequired: new(true), Clear: []string{colConsentWording}}, colConsentWording},
		"a pause":                      {browser, UpdateInput{State: new(StatePaused)}, ""},
		"a policy link with no host":   {browser, UpdateInput{PrivacyNoticeURL: new("https://:443/privacy")}, colPrivacyNoticeURL},
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
		LeadOrigin: new("website"), RedirectURL: new("https://www.example.com/thanks"),
		SuccessMessage: new("Thanks"), ConsentPolicyVersion: new("v1"),
		PrivacyNotice: new("We answer your request."), PrivacyNoticeURL: new("https://www.example.com/privacy"),
	}
	in := UpdateInput{
		Name: new("Renamed"), SuccessMessage: new("Danke"),
		Clear: []string{
			colLeadOrigin, colOwnerID, colRedirectURL, colConsentPolicyVersion, colConsentWording, colSuccessMessage,
			colPrivacyNotice, colPrivacyNoticeURL,
		},
	}
	got, err := in.merged(form)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.LeadOrigin != nil || got.RedirectURL != nil ||
		got.ConsentPolicyVersion != nil || got.SuccessMessage != nil || got.PrivacyNotice != nil || got.PrivacyNoticeURL != nil {
		t.Fatalf("merged = %+v, want every cleared setting empty", got)
	}
}

func TestTheInstallationMintsEverySecretABuilderLetsSomeoneTypeIn(t *testing.T) {
	pasted := new("a-builder-secret-of-length")
	for name, tc := range map[string]struct {
		sender string
		pasted *string
		mints  bool
	}{
		"a server form":           {SenderServer, nil, true},
		"typeform without one":    {SenderTypeform, nil, true},
		"tally with an empty one": {SenderTally, new(""), true},
		"tally with a pasted one": {SenderTally, pasted, false},
		"webflow, which issues":   {SenderWebflow, nil, false},
		"a browser form":          {SenderBrowser, nil, false},
	} {
		if got := MintsSecret(tc.sender, tc.pasted); got != tc.mints {
			t.Errorf("%s: mints = %v, want %v", name, got, tc.mints)
		}
	}
}
