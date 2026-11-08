// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package doctext

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return raw
}

// archive builds a zip of the given entries, in order.
func archive(t *testing.T, entries ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		f, err := w.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAWorkbookReadsEverySheetWithNamesBesideTheirSalaries(t *testing.T) {
	got, err := Read(context.Background(), fixture(t, "salaries.xlsx"), 2000)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, line := range []string{
		"Sheet: Summary\nCost plus statement\n",
		"Sheet: Cost+\nEmployee | Role | Gross salary\n",
		"Minh Tran | Senior developer | 2450\n",
		"Linh Pham | Developer | 1680",
	} {
		if !strings.Contains(got.Text, line) {
			t.Errorf("excerpt lacks %q:\n%s", line, got.Text)
		}
	}
	if strings.Index(got.Text, "Sheet: Summary") > strings.Index(got.Text, "Sheet: Cost+") {
		t.Error("sheets are not in the order the workbook declares, which is not the order of the relationships file")
	}
	if strings.Contains(got.Text, "1400*1.2") {
		t.Error("a formula reached the excerpt; a cell reads as its value")
	}
	if got.Truncated {
		t.Error("a workbook that fits is reported as truncated")
	}
}

func TestADocumentReadsItsParagraphs(t *testing.T) {
	got, err := Read(context.Background(), fixture(t, "letter.docx"), 2000)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := "Termination agreement\nEmployee: Ada Lovelace\tNotice: 3 months"; got.Text != want {
		t.Errorf("excerpt = %q, want %q", got.Text, want)
	}
}

func TestAnExcerptStopsAtItsBudgetAndSaysSo(t *testing.T) {
	got, err := Read(context.Background(), fixture(t, "salaries.xlsx"), 40)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n := utf8.RuneCountInString(got.Text); n > 40 {
		t.Errorf("excerpt holds %d runes, budget 40", n)
	}
	if !got.Truncated {
		t.Error("a cut excerpt is not marked truncated, so a reader would believe it saw the whole file")
	}
}

func TestATextFileIsItsOwnExcerpt(t *testing.T) {
	got, err := Read(context.Background(), []byte("name,salary\nAda,5000\n"), 100)
	if err != nil || got.Text != "name,salary\nAda,5000" || got.Truncated {
		t.Errorf("Read = %+v, %v; want the CSV text, untruncated", got, err)
	}
}

func TestAWorkbookWithoutRelationshipsFallsBackToItsWorksheets(t *testing.T) {
	raw := archive(t,
		[2]string{"xl/workbook.xml", `<workbook><sheets/></workbook>`},
		[2]string{"xl/worksheets/sheet1.xml", `<worksheet><sheetData><row><c t="inlineStr"><is><t>Ada</t></is></c><c><v>5000</v></c></row></sheetData></worksheet>`},
	)
	got, err := Read(context.Background(), raw, 100)
	if err != nil || !strings.Contains(got.Text, "Ada | 5000") {
		t.Errorf("Read = %+v, %v; want the worksheet's row", got, err)
	}
}

func TestAnArchiveThatInflatesPastItsBoundIsRefused(t *testing.T) {
	// Twenty MiB of one repeated cell compresses to a few kilobytes: the
	// declared size is the archive's claim, and only the read budget holds.
	bomb := `<worksheet><sheetData><row><c t="inlineStr"><is><t>` + strings.Repeat("A", 20<<20) + `</t></is></c></row></sheetData></worksheet>`
	raw := archive(t, [2]string{"xl/workbook.xml", `<workbook/>`}, [2]string{"xl/worksheets/sheet1.xml", bomb})
	if len(raw) > 1<<20 {
		t.Fatalf("the fixture is %d bytes; it no longer tests inflation", len(raw))
	}
	if _, err := Read(context.Background(), raw, 1500); !errors.Is(err, ErrUnreadable) {
		t.Errorf("Read = %v, want ErrUnreadable for an archive past its inflation bound", err)
	}
}

func TestAnArchiveOfTooManyEntriesIsRefused(t *testing.T) {
	entries := [][2]string{{"xl/workbook.xml", `<workbook/>`}}
	for i := range maxEntries {
		entries = append(entries, [2]string{fmt.Sprintf("xl/media/%d.xml", i), "x"})
	}
	if _, err := Read(context.Background(), archive(t, entries...), 100); !errors.Is(err, ErrUnreadable) {
		t.Errorf("Read = %v, want ErrUnreadable past %d entries", err, maxEntries)
	}
}

func TestFilesThatAreNotDocumentsAreUnsupported(t *testing.T) {
	for name, raw := range map[string][]byte{
		"a zip of photos":   archive(t, [2]string{"IMG_1.jpg", "\xff\xd8\xff"}),
		"a legacy xls":      {0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0, 0},
		"a broken zip head": []byte("PK\x03\x04garbage"),
	} {
		_, err := Read(context.Background(), raw, 100)
		if !errors.Is(err, ErrUnsupported) && !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: Read = %v, want unsupported or unreadable", name, err)
		}
	}
}

func TestAnEmptyWorkbookStatesNoText(t *testing.T) {
	raw := archive(t, [2]string{"xl/workbook.xml", `<workbook/>`}, [2]string{"xl/worksheets/sheet1.xml", `<worksheet><sheetData/></worksheet>`})
	if _, err := Read(context.Background(), raw, 100); !errors.Is(err, ErrNoText) {
		t.Errorf("Read = %v, want ErrNoText", err)
	}
}

func TestACancelledReadStopsWithTheCallersError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, fixture(t, "salaries.xlsx"), 100); !errors.Is(err, context.Canceled) {
		t.Errorf("Read = %v, want the caller's cancellation", err)
	}
}
