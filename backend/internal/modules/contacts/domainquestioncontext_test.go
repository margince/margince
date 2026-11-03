// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAQuestionNamesOnlyTheSendersItsReaderMaySee(t *testing.T) {
	mail := ids.NewV7()
	seen, hidden := ids.NewV7(), ids.NewV7()
	var out DomainQuestionContext
	domainSenders{
		{activity: mail, address: "hidden@alias.example", contact: &hidden, name: "Hidden Sender"},
		{activity: mail, address: "alex@alias.example", contact: &seen, name: "Alex Example", visible: true},
		{activity: ids.NewV7(), address: "alex@alias.example", contact: &seen, name: "Alex Example", visible: true},
	}.fill(&out)

	if len(out.Contacts) != 1 || out.Contacts[0].ID != seen || out.Contacts[0].Name != "Alex Example" {
		t.Fatalf("contacts = %+v, want only Alex, once", out.Contacts)
	}
	if out.HiddenSenders {
		t.Error("hidden_senders is set although a visible sender is named")
	}
	if out.OriginActivityID == nil || *out.OriginActivityID != mail || out.SenderAddress != "hidden@alias.example" {
		t.Fatalf("origin = %v from %q, want the newest mail and its sender", out.OriginActivityID, out.SenderAddress)
	}
}

func TestAQuestionFromHiddenSendersSaysSoWithoutNamingThem(t *testing.T) {
	hidden := ids.NewV7()
	var out DomainQuestionContext
	domainSenders{{activity: ids.NewV7(), address: "p@alias.example", contact: &hidden, name: "Hidden Sender"}}.fill(&out)
	if len(out.Contacts) != 0 || !out.HiddenSenders {
		t.Fatalf("context = %+v, want no names and hidden_senders set", out)
	}
}

func TestAQuestionNamesAtMostThreeSenders(t *testing.T) {
	var list domainSenders
	for range 5 {
		contact := ids.NewV7()
		list = append(list, domainSender{activity: ids.NewV7(), address: "x@alias.example", contact: &contact, name: "Someone", visible: true})
	}
	var out DomainQuestionContext
	list.fill(&out)
	if len(out.Contacts) != domainQuestionContactLimit {
		t.Fatalf("%d contacts named, want %d", len(out.Contacts), domainQuestionContactLimit)
	}
}

func TestASuggestionNeedsOneVisibleCompany(t *testing.T) {
	acme, other := ids.NewV7(), ids.NewV7()
	names := map[ids.UUID]string{acme: "Acme GmbH", other: "Other AG"}
	cases := []struct {
		name    string
		offered []domainSuggestionCandidate
		names   map[ids.UUID]string
		want    *DomainCompanySuggestion
	}{
		{name: "nothing offered", names: names},
		{
			name:    "two sources agree",
			offered: []domainSuggestionCandidate{{company: acme, basis: SuggestionRedirectTarget}, {company: acme, basis: SuggestionContactEmployer}},
			names:   names,
			want:    &DomainCompanySuggestion{CompanyID: acme, Name: "Acme GmbH", Basis: SuggestionRedirectTarget},
		},
		{
			name:    "two companies",
			offered: []domainSuggestionCandidate{{company: acme, basis: SuggestionContactEmployer}, {company: other, basis: SuggestionContactEmployer}},
			names:   names,
		},
		{
			name:    "a hidden twin does not hide a visible employer",
			offered: []domainSuggestionCandidate{{company: acme, basis: SuggestionContactEmployer}, {company: ids.NewV7(), basis: SuggestionNameTwin}},
			names:   names,
			want:    &DomainCompanySuggestion{CompanyID: acme, Name: "Acme GmbH", Basis: SuggestionContactEmployer},
		},
		{
			name:    "the one company is hidden",
			offered: []domainSuggestionCandidate{{company: acme, basis: SuggestionNameTwin}},
			names:   map[ids.UUID]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := soleSuggestion(tc.offered, tc.names)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("suggestion = %+v, want %+v", got, tc.want)
			}
		})
	}
}
