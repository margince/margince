// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// reportedSignature is a sign-off, a signature, a company block and a legal
// footer: more lines below the name than a short tail window holds.
var reportedSignature = []string{
	"Hallo Herr Schmidt,",
	"danke, der Termin passt.",
	"Mit freundlichen Grüßen",
	"Anna Weber",
	"Leiterin Einkauf",
	"T +49 30 1234567",
	"M +49 151 2345678",
	"anna.weber@example.de",
	"Beispiel Handels GmbH",
	"Musterstraße 12",
	"10115 Berlin",
	"Deutschland",
	"www.beispiel-handels.de",
	"Geschäftsführer: Max Mustermann",
	"Sitz der Gesellschaft: Berlin",
	"Amtsgericht Charlottenburg HRB 123456",
	"Diese E-Mail enthält vertrauliche Informationen.",
	"Wenn Sie nicht der richtige Adressat sind, informieren Sie den Absender.",
	"USt-IdNr.: DE123456789",
	"-----Ursprünglicher Termin-----",
}

func TestTheSignatureWindowOpensAtTheSignOff(t *testing.T) {
	t.Parallel()
	got := SignatureWindow(reportedSignature, 25)
	if got[0] != "Mit freundlichen Grüßen" || !slices.Contains(got, "Leiterin Einkauf") {
		t.Fatalf("window = %q, want it to open at the sign-off and hold the title", got)
	}
}

func TestALongSignatureKeepsItsHeadAndDropsItsFooter(t *testing.T) {
	t.Parallel()
	got := SignatureWindow(reportedSignature, 6)
	want := reportedSignature[2:8]
	if !slices.Equal(got, want) {
		t.Fatalf("window = %q, want the sign-off and the five lines under it %q", got, want)
	}
}

func TestTheLastSignOffOpensTheWindowNotAThanksAboveTheMessage(t *testing.T) {
	t.Parallel()
	lines := []string{"Thanks!"}
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("prose line %d", i))
	}
	lines = append(lines, "Best regards,", "Jane Smith", "Head of Sales")
	got := SignatureWindow(lines, 25)
	if strings.Join(got, "\n") != "Best regards,\nJane Smith\nHead of Sales" {
		t.Fatalf("window = %q, want it to open at the closing sign-off", got)
	}
}

func TestASigDashAboveTheSignOffOpensTheWindow(t *testing.T) {
	t.Parallel()
	lines := []string{"See you then.", "-- ", "Jane Smith", "Head of Sales", "Thanks"}
	if got := SignatureWindow(lines, 25); got[0] != "-- " {
		t.Fatalf("window = %q, want it to open at the sig-dash", got)
	}
}

func TestWithoutABoundaryTheWindowIsTheTail(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("prose line %d", i))
	}
	got := SignatureWindow(lines, 25)
	if len(got) != 25 || got[24] != "prose line 39" {
		t.Fatalf("window = %q, want the last 25 lines", got)
	}
	if got := SignatureWindow(nil, 25); len(got) != 0 {
		t.Fatalf("window of no lines = %q, want none", got)
	}
}
