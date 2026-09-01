// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The cards real exporters actually emit, not the ones the RFC draws.
//
// Every case here is a shape taken from a working address book — Apple's group
// prefixes, Outlook's quoted-printable, a 2.1-era bare type parameter, the
// query junk a browser leaves on a URL. A parser tested only against the
// specification's examples passes and then meets a file from a laptop.

import (
	"strings"
	"testing"
)

func TestParseVCardsReadsTheCardsExportersWrite(t *testing.T) {
	cases := []struct {
		name string
		card string
		want VCardEntry
	}{
		{
			name: "a plain 3.0 card",
			card: "BEGIN:VCARD\nVERSION:3.0\nFN:Dana Buyer\nORG:Acme GmbH\nTITLE:VP Finance\n" +
				"EMAIL;TYPE=WORK:dana@acme.example\nTEL;TYPE=CELL:+49 30 111\nEND:VCARD\n",
			want: VCardEntry{
				FullName: "Dana Buyer", Organization: "Acme GmbH", Title: "VP Finance",
				Emails: []VCardChannel{{Value: "dana@acme.example", Kind: "work"}},
				Phones: []VCardChannel{{Value: "+49 30 111", Kind: "mobile"}},
			},
		},
		{
			name: "an ORG with departments keeps only the company",
			// The components after the first are departments, and a department
			// is not an employer.
			card: "BEGIN:VCARD\nFN:Sam Rep\nORG:Globex;Sales;EMEA\nEND:VCARD\n",
			want: VCardEntry{FullName: "Sam Rep", Organization: "Globex"},
		},
		{
			name: "a card with no FN falls back to the structured name",
			card: "BEGIN:VCARD\nN:Fischer;Lena;;Dr.;\nEND:VCARD\n",
			// Given then family, in reading order; the prefix is dropped
			// because "Dr." is not part of a name this product matches on.
			want: VCardEntry{FullName: "Lena Fischer"},
		},
		{
			name: "Apple's group prefix names the same property",
			card: "BEGIN:VCARD\nFN:Ada Lovelace\nitem1.TEL;type=pref:+44 20 7946\n" +
				"item1.X-ABLabel:main\nEND:VCARD\n",
			want: VCardEntry{
				FullName: "Ada Lovelace",
				// `pref` says which is primary, not what kind it is, so the
				// reader looks past it and lands on the work default.
				Phones: []VCardChannel{{Value: "+44 20 7946", Kind: "work"}},
			},
		},
		{
			name: "a 2.1 bare type parameter",
			card: "BEGIN:VCARD\nVERSION:2.1\nFN:Jean Doe\nTEL;HOME:+33 1 4567\nEND:VCARD\n",
			want: VCardEntry{
				FullName: "Jean Doe",
				Phones:   []VCardChannel{{Value: "+33 1 4567", Kind: "home"}},
			},
		},
		{
			name: "a folded line is one value",
			// A long value is split across lines with a leading space, and a
			// reader taking raw lines stores half an address.
			card: "BEGIN:VCARD\nFN:Wilhelmina Featherstonehaugh-\n Marchbanks\nEND:VCARD\n",
			want: VCardEntry{FullName: "Wilhelmina Featherstonehaugh-Marchbanks"},
		},
		{
			name: "escaped punctuation arrives as punctuation",
			card: `BEGIN:VCARD` + "\n" + `FN:Acme\, Inc.` + "\n" + `ORG:Acme\; Holdings` + "\n" + `END:VCARD` + "\n",
			want: VCardEntry{FullName: "Acme, Inc.", Organization: "Acme; Holdings"},
		},
		{
			name: "quoted-printable is decoded",
			card: "BEGIN:VCARD\nFN;ENCODING=QUOTED-PRINTABLE:J=C3=BCrgen M=C3=BCller\nEND:VCARD\n",
			want: VCardEntry{FullName: "Jürgen Müller"},
		},
		{
			name: "a structured address becomes one line",
			card: "BEGIN:VCARD\nFN:Postal Person\nADR;TYPE=WORK:;;Hauptstr. 1;Berlin;;10115;Germany\nEND:VCARD\n",
			// The post-office box and extended address are dropped; what
			// remains is what a record shows.
			want: VCardEntry{FullName: "Postal Person", Address: "Hauptstr. 1, Berlin, 10115, Germany"},
		},
		{
			name: "a home email is personal",
			card: "BEGIN:VCARD\nFN:Home Contact\nEMAIL;TYPE=HOME:me@home.example\nEND:VCARD\n",
			want: VCardEntry{
				FullName: "Home Contact",
				Emails:   []VCardChannel{{Value: "me@home.example", Kind: "personal"}},
			},
		},
		{
			name: "a URL with a colon in a quoted parameter is not split early",
			card: `BEGIN:VCARD` + "\n" + `FN:Linked Person` + "\n" +
				`URL;TYPE="work:main":https://example.com/team/jdoe` + "\n" + `END:VCARD` + "\n",
			want: VCardEntry{FullName: "Linked Person", URL: "https://example.com/team/jdoe"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseVCards(strings.NewReader(tc.card))
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("cards = %d, want 1", len(got))
			}
			assertVCardEntry(t, got[0], tc.want)
		})
	}
}

func TestParseVCardsKeepsAnEscapedSemicolonInsideAComponent(t *testing.T) {
	// A structured value is split on its UNESCAPED semicolons. Decoding first
	// would turn an escaped one inside a family name into a component
	// boundary, and the person's own identity — what dedupe matches on —
	// would be assembled wrong.
	card := `BEGIN:VCARD` + "\n" + `N:Smith\;Jones;Alice;;;` + "\n" + `END:VCARD` + "\n"
	got, err := ParseVCards(strings.NewReader(card))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got[0].FullName != "Alice Smith;Jones" {
		t.Errorf("full name = %q, want %q", got[0].FullName, "Alice Smith;Jones")
	}
}

func TestParseVCardsRefusesAFileTooLargeRatherThanReadingItsPrefix(t *testing.T) {
	// The bound must REFUSE, not truncate: a prefix that parses is an import
	// of part of an address book, and nobody can tell which part.
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\nFN:Big Card\nNOTE:")
	b.WriteString(strings.Repeat("x", vcardMaxBytes))
	b.WriteString("\nEND:VCARD\n")
	if _, err := ParseVCards(strings.NewReader(b.String())); err == nil {
		t.Fatal("an oversized file parsed, want a refusal")
	}
}

func TestParseVCardsReadsEveryCardInAMultiCardFile(t *testing.T) {
	file := "BEGIN:VCARD\nFN:First Person\nEND:VCARD\n" +
		"BEGIN:VCARD\nFN:Second Person\nEND:VCARD\n" +
		"BEGIN:VCARD\nFN:Third Person\nEND:VCARD\n"
	got, err := ParseVCards(strings.NewReader(file))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("cards = %d, want 3", len(got))
	}
	for i, want := range []string{"First Person", "Second Person", "Third Person"} {
		if got[i].FullName != want {
			t.Errorf("card %d full name = %q, want %q", i, got[i].FullName, want)
		}
	}
}

func TestParseVCardsRefusesAFileItCannotRead(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		// An import that quietly drops a person is worse than one that
		// refuses: the reader has no way to notice who is missing.
		{"a card that never ends", "BEGIN:VCARD\nFN:Truncated Person\n"},
		// THE SAME MALFORMATION ONE CARD FROM THE END. The card that never
		// ended is in the MIDDLE, so the next BEGIN arrives before the file
		// does — and starting a card over an open one dropped the open one and
		// everything on it. Three cards in, two rows out, and a 200 saying so.
		{
			"a card that never ends, in the middle of the file",
			"BEGIN:VCARD\nFN:Alpha\nEND:VCARD\n" +
				"BEGIN:VCARD\nFN:Beta\n" +
				"BEGIN:VCARD\nFN:Gamma\nEND:VCARD\n",
		},
		{"an END with nothing open", "END:VCARD\nBEGIN:VCARD\nFN:Late\nEND:VCARD\n"},
		// A DELIMITER NAMING SOMETHING ELSE. Read past, the marker is skipped
		// and the properties under it are not: a calendar's fields land on
		// whichever card is open, or vanish where none is, and the file is
		// reported as imported either way.
		{
			"a card wrapped in something that is not a vCard",
			"BEGIN:VCALENDAR\nBEGIN:VCARD\nFN:Inside A Calendar\nEND:VCARD\nEND:VCALENDAR\n",
		},
		{
			"a stray end marker for something else",
			"BEGIN:VCARD\nFN:Fine\nEND:VCARD\nEND:VCALENDAR\n",
		},
		{"a file with no card at all", "this is not a vCard\n"},
		{"an empty file", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseVCards(strings.NewReader(tc.file)); err == nil {
				t.Fatal("parsing succeeded, want a refusal")
			}
		})
	}
}

func TestParseVCardsKeepsEveryChannelACardStates(t *testing.T) {
	card := "BEGIN:VCARD\nFN:Many Channels\n" +
		"EMAIL;TYPE=WORK:work@example.com\nEMAIL;TYPE=HOME:home@example.com\n" +
		"TEL;TYPE=WORK:+1 555 0100\nTEL;TYPE=CELL:+1 555 0111\nEND:VCARD\n"
	got, err := ParseVCards(strings.NewReader(card))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// Both of each: a card listing two addresses is a person reachable two
	// ways, and keeping one would drop a fact the person themselves stated.
	if len(got[0].Emails) != 2 {
		t.Errorf("emails = %v, want both", got[0].Emails)
	}
	if len(got[0].Phones) != 2 {
		t.Errorf("phones = %v, want both", got[0].Phones)
	}
}

func assertVCardEntry(t *testing.T, got, want VCardEntry) {
	t.Helper()
	if got.FullName != want.FullName {
		t.Errorf("full name = %q, want %q", got.FullName, want.FullName)
	}
	if got.Organization != want.Organization {
		t.Errorf("organization = %q, want %q", got.Organization, want.Organization)
	}
	if got.Title != want.Title {
		t.Errorf("title = %q, want %q", got.Title, want.Title)
	}
	if got.URL != want.URL {
		t.Errorf("url = %q, want %q", got.URL, want.URL)
	}
	if got.Address != want.Address {
		t.Errorf("address = %q, want %q", got.Address, want.Address)
	}
	assertChannels(t, "emails", got.Emails, want.Emails)
	assertChannels(t, "phones", got.Phones, want.Phones)
}

func assertChannels(t *testing.T, what string, got, want []VCardChannel) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %+v, want %+v", what, i, got[i], want[i])
		}
	}
}
