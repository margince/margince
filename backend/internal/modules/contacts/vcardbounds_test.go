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
	atCap := strings.Repeat("ü", ImportedNameLimit)
	if reason := cardRefusal(parseOneCard(t, "BEGIN:VCARD\nFN:"+atCap+"\nEND:VCARD\n")); reason != "" {
		t.Fatalf("reason = %q, want none for a name at the bound", reason)
	}
	reason := cardRefusal(parseOneCard(t, "BEGIN:VCARD\nFN:"+atCap+"x\nEND:VCARD\n"))
	want := "The name is " + strconv.Itoa(ImportedNameLimit+1) + " characters. A contact name holds at most " +
		strconv.Itoa(ImportedNameLimit) + "."
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestOverlongImportedNameNamesThePartPastTheBound(t *testing.T) {
	over := strings.Repeat("a", ImportedNameLimit+1)
	for _, column := range []string{contactNameColumn, columnFirstName, columnLastName} {
		got, length, refused := OverlongImportedName(map[string]string{contactNameColumn: "Anna", column: over})
		if !refused || got != column || length != ImportedNameLimit+1 {
			t.Errorf("%s: got %q, %d, %v; want it refused at %d", column, got, length, refused, ImportedNameLimit+1)
		}
	}
	if _, _, refused := OverlongImportedName(map[string]string{contactNameColumn: "  " + over[1:] + "  "}); refused {
		t.Errorf("a name at the bound with surrounding spaces was refused; the bound counts the trimmed name")
	}
}

// The full name a join stores obeys the bound its parts do: 100 and 99 join to
// the bound, 150 and 150 join past it and answer full_name.
func TestOverlongImportedNameRefusesPartsThatJoinPastTheBound(t *testing.T) {
	fits := map[string]string{columnFirstName: strings.Repeat("a", 100), columnLastName: strings.Repeat("b", 99)}
	if column, length, refused := OverlongImportedName(fits); refused {
		t.Errorf("got %q, %d refused; want a %d-character join admitted", column, length, ImportedNameLimit)
	}
	over := map[string]string{columnFirstName: strings.Repeat("a", 150), columnLastName: strings.Repeat("b", 150)}
	if column, length, refused := OverlongImportedName(over); !refused || column != contactNameColumn || length != 301 {
		t.Errorf("got %q, %d, %v; want full_name refused at 301", column, length, refused)
	}
}

// An imported update whose part joins to a stored part past the bound is
// refused on full_name; one the store leaves unchanged is not asked.
func TestAnImportedUpdateJoiningPastTheBoundIsRefused(t *testing.T) {
	last := strings.Repeat("b", 150)
	current := crmcontracts.Contact{FullName: "Ada " + last, FirstName: new("Ada"), LastName: &last}
	in := updateAsWritten(current, UpdateContactInput{FirstName: new(strings.Repeat("a", 150))}, nil)
	err := checkImportedFullName(current, in)
	want := "full_name holds at most " + strconv.Itoa(ImportedNameLimit) + " characters; this one has 301"
	if _, ok := errors.AsType[*httperr.DetailedError](err); !ok || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if err := checkImportedFullName(current, UpdateContactInput{FullName: &current.FullName}); err != nil {
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
