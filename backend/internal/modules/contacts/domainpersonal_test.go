// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestDomainLooksPersonalNeedsEveryKnownContactToExplainTheLabel(t *testing.T) {
	cases := []struct {
		name     string
		label    string
		contacts []DomainContact
		want     bool
	}{
		{
			// kestner.example: the label is the sender's surname outright.
			name:     "the surname is the domain",
			label:    "kestner",
			contacts: []DomainContact{{FullName: "Sebastian Kestner", EmailLocals: []string{"sebastian"}}},
			want:     true,
		},
		{
			// rowanmarsh.example: the header display name is "Ines Marsh" and
			// explains only half of it. The local part explains the rest, which
			// is why the address and not just the name has to be looked at.
			name:     "the local part plus the surname is the domain",
			label:    "rowanmarsh",
			contacts: []DomainContact{{FullName: "Ines Marsh", EmailLocals: []string{"rowan"}}},
			want:     true,
		},
		{
			name:     "first and last run together",
			label:    "olivia-larsen",
			contacts: []DomainContact{{FullName: "Olivia Larsen", EmailLocals: []string{"info"}}},
			want:     true,
		},
		{
			name:     "last and first run together",
			label:    "hausen-kai",
			contacts: []DomainContact{{FullName: "Kai Hausen", EmailLocals: []string{"kai"}}},
			want:     true,
		},
		{
			name:     "a transliterated surname does not match the accented name",
			label:    "mueller",
			contacts: []DomainContact{{FullName: "Christina Müller", EmailLocals: []string{"cm"}}},
			// "mueller" is a TRANSLITERATION of Müller, not an unaccenting of
			// it — normalizeName yields "muller". Whether to fold German
			// digraphs is a question for the whole dedupe metric, not something
			// to answer once, here.
			want: false,
		},
		{
			name:     "a company domain nobody's name explains",
			label:    "basecom",
			contacts: []DomainContact{{FullName: "Martin Weiss", EmailLocals: []string{"m.wortmann"}}},
			want:     false,
		},
		{
			// Two unrelated family names on one domain is what a company IS.
			// The employer stapled onto each display name must not read as
			// their surname, or both would "match" and the company would be
			// refused its company.
			name:  "a shared domain is a company, and a stapled employer is not a surname",
			label: "tvpartner",
			contacts: []DomainContact{
				{FullName: "Tomas Vidal - TVPartner", EmailLocals: []string{"guido"}},
				{FullName: "Karin Vogt - TVPartner", EmailLocals: []string{"annett"}},
			},
			want: false,
		},
		{
			// A mailbox named after the company is not a contact, and it is the
			// one case where the local part alone equals the label.
			name:     "a role address on its own domain is not a contact",
			label:    "tvpartner",
			contacts: []DomainContact{{FullName: "Tomas Vidal", EmailLocals: []string{"tvpartner"}}},
			want:     false,
		},
		{
			name:     "an employer in parentheses is not a surname either",
			label:    "cloud-atlas",
			contacts: []DomainContact{{FullName: "Nikos Adamos (Cloud Atlas)", EmailLocals: []string{"stergios"}}},
			want:     false,
		},
		{
			name:     "nobody known is not evidence of anything",
			label:    "kestner",
			contacts: nil,
			want:     false,
		},
		{
			name:     "an empty label answers no rather than matching everything",
			label:    "",
			contacts: []DomainContact{{FullName: "Sebastian Kestner", EmailLocals: []string{"sebastian"}}},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DomainLooksPersonal(tc.label, tc.contacts); got != tc.want {
				t.Errorf("DomainLooksPersonal(%q, %v) = %v, want %v", tc.label, tc.contacts, got, tc.want)
			}
		})
	}
}

func TestDomainLooksPersonalMisjudgesAnEponymousAgency(t *testing.T) {
	// baumert.example is Alexander Baumert's agency and the heuristic cannot tell
	// it from a personal domain — the label IS his surname. It answers yes, and
	// the company is lost.
	//
	// That is why this heuristic runs ONLY when no site could be read: for
	// baumert.example a site loads, states a company, and the crawl answers first.
	// The test pins the limit so nobody later mistakes the heuristic for a
	// standalone rule.
	if !DomainLooksPersonal("baumert", []DomainContact{{FullName: "Alexander Baumert", EmailLocals: []string{"alexander"}}}) {
		t.Fatal("the heuristic is expected to misjudge an eponymous agency; if it no longer does, the comment above is stale")
	}
}
