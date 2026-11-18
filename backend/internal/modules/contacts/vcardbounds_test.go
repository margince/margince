// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
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
