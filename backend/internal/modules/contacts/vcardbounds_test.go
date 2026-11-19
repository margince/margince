// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func parseOneCard(t *testing.T, file string) VCardEntry {
	t.Helper()
	entries, err := ParseVCards(strings.NewReader(file))
	if err != nil || len(entries) != 1 {
		t.Fatalf("parsing = %+v, %v; want one card", entries, err)
	}
	return entries[0]
}

// A card name is held to the same bound as a file's name cell, in characters.
func TestACardNamePastTheImportBoundIsRefusedWithItsLength(t *testing.T) {
	atCap := strings.Repeat("ü", ContactNameLimit)
	if reason := cardRefusal(parseOneCard(t, "BEGIN:VCARD\nFN:"+atCap+"\nEND:VCARD\n")); reason != "" {
		t.Fatalf("reason = %q, want none for a name at the bound", reason)
	}
	reason := cardRefusal(parseOneCard(t, "BEGIN:VCARD\nFN:"+atCap+"x\nEND:VCARD\n"))
	want := "The name is " + strconv.Itoa(ContactNameLimit+1) + " characters. A contact name holds at most " +
		strconv.Itoa(ContactNameLimit) + "."
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestOverlongContactNameNamesThePartPastTheBound(t *testing.T) {
	over := strings.Repeat("a", ContactNameLimit+1)
	for _, column := range []string{contactNameColumn, columnFirstName, columnLastName} {
		got, length, refused := OverlongContactName(map[string]string{contactNameColumn: "Anna", column: over})
		if !refused || got != column || length != ContactNameLimit+1 {
			t.Errorf("%s: got %q, %d, %v; want it refused at %d", column, got, length, refused, ContactNameLimit+1)
		}
	}
	if _, _, refused := OverlongContactName(map[string]string{contactNameColumn: "  " + over[1:] + "  "}); refused {
		t.Errorf("a name at the bound with surrounding spaces was refused; the bound counts the trimmed name")
	}
}

// The full name a join stores obeys the bound its parts do. With the label
// limit of 120, 60 and 59 join to the bound; 80 and 80 join past it at 161.
func TestOverlongContactNameRefusesPartsThatJoinPastTheBound(t *testing.T) {
	first, last := ContactNameLimit/2, ContactNameLimit-ContactNameLimit/2-1
	fits := map[string]string{columnFirstName: strings.Repeat("a", first), columnLastName: strings.Repeat("b", last)}
	if column, length, refused := OverlongContactName(fits); refused {
		t.Errorf("got %q, %d refused; want a %d-character join admitted", column, length, ContactNameLimit)
	}
	part := ContactNameLimit * 2 / 3
	over := map[string]string{columnFirstName: strings.Repeat("a", part), columnLastName: strings.Repeat("b", part)}
	if column, length, refused := OverlongContactName(over); !refused || column != contactNameColumn || length != 2*part+1 {
		t.Errorf("got %q, %d, %v; want full_name refused at %d", column, length, refused, 2*part+1)
	}
}

// An update whose part joins to a stored part past the bound is refused on
// full_name. A full name the update leaves unchanged is not asked.
func TestAnUpdateJoiningPastTheBoundIsRefused(t *testing.T) {
	part := ContactNameLimit * 2 / 3
	last := strings.Repeat("b", part)
	current := crmcontracts.Contact{FullName: "Ada " + last, FirstName: new("Ada"), LastName: &last}
	in := updateAsWritten(current, UpdateContactInput{FirstName: new(strings.Repeat("a", part))}, nil)
	err := checkChangedContactNames(current, in)
	want := "full_name holds at most " + strconv.Itoa(ContactNameLimit) + " characters; this one has " + strconv.Itoa(2*part+1)
	if _, ok := errors.AsType[*httperr.DetailedError](err); !ok || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if err := checkChangedContactNames(current, UpdateContactInput{FullName: &current.FullName}); err != nil {
		t.Errorf("err = %v, want an unchanged full name admitted", err)
	}
}

// A cut title the record took is told to the reader, after any other warning.
// A title the record did not take, or one that fit, says nothing.
func TestACutTitleIsReportedOnlyWhenTheRecordTookIt(t *testing.T) {
	limit := textbounds.LimitOf("public.contact.title")
	long := parseOneCard(t, "BEGIN:VCARD\nFN:Ada\nTITLE:"+strings.Repeat("t", limit+80)+"\nEND:VCARD\n")
	took := []VCardField{{Field: fieldTitle, Action: VCardFieldReplaced}}
	want := "The title is " + strconv.Itoa(limit+80) + " characters, so only its first " + strconv.Itoa(limit) + " were imported."
	if got := withTitleNote("", long, took); got != want {
		t.Errorf("note = %q, want %q", got, want)
	}
	if got := withTitleNote("No channel.", long, took); got != "No channel. "+want {
		t.Errorf("note = %q, want it after the existing warning", got)
	}
	if got := withTitleNote("", long, []VCardField{{Field: fieldTitle, Action: VCardFieldKept}}); got != "" {
		t.Errorf("note = %q, want none when the record kept its own title", got)
	}
	short := parseOneCard(t, "BEGIN:VCARD\nFN:Ada\nTITLE:CTO\nEND:VCARD\n")
	if got := withTitleNote("", short, took); got != "" {
		t.Errorf("note = %q, want none for a title that fit", got)
	}
}
