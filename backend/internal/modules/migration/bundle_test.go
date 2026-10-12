// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestABundleMatchesEachFileToItsTypeByName(t *testing.T) {
	b, err := OpenBundle(zipOf(t, map[string]string{
		"export/Companies.CSV":     "ref,name\n",
		"export/contacts.csv":      "ref,email\n",
		"lead.csv":                 "email\n",
		"README.txt":               "how to",
		"__MACOSX/._companies.csv": "junk",
		".DS_Store":                "junk",
		"products.csv":             "Product\n",
		"deals.csv":                "ref,name\n",
	}), 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range b.Parts {
		got[p.Entry] = p.Object
	}
	want := map[string]string{"export/Companies.CSV": ObjectCompany, "export/contacts.csv": ObjectContact, "lead.csv": ObjectLead}
	if len(got) != len(want) {
		t.Fatalf("parts = %v, want %v", got, want)
	}
	for entry, object := range want {
		if got[entry] != object {
			t.Errorf("%s matched %q, want %q", entry, got[entry], object)
		}
	}
	ignored := map[string]string{}
	for _, i := range b.Ignored {
		ignored[i.Entry] = i.Reason
	}
	if ignored["products.csv"] != IgnoredUnknownName || ignored["deals.csv"] != IgnoredNotYetImportable || len(ignored) != 2 {
		t.Fatalf("ignored = %v, want products.csv as unknown and deals.csv as not yet importable, and nothing else listed", ignored)
	}
}

func TestABundleOfMoreThanTwentyFilesIsRefused(t *testing.T) {
	files := map[string]string{}
	for i := range MaxBundleEntries + 1 {
		files[strings.Repeat("x", i+1)+".csv"] = "a\n"
	}
	if _, err := OpenBundle(zipOf(t, files), 10<<20); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v, want ErrBundleRefused", err)
	}
}

func TestABundleWhoseDeclaredSizesPassTenTimesTheCapIsRefused(t *testing.T) {
	body := zipOf(t, map[string]string{"companies.csv": strings.Repeat("a,b\n", 300)})
	if _, err := OpenBundle(body, 100); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v, want ErrBundleRefused for 1200 bytes against a 100-byte cap", err)
	}
}

func TestTwoFilesForOneTypeAreRefusedNamingBoth(t *testing.T) {
	_, err := OpenBundle(zipOf(t, map[string]string{"companies.csv": "a\n", "Company.csv": "a\n"}), 10<<20)
	if !errors.Is(err, ErrBundleRefused) || !strings.Contains(err.Error(), "companies.csv") || !strings.Contains(err.Error(), "Company.csv") {
		t.Fatalf("err = %v, want a refusal naming both files", err)
	}
}

func TestAnEntryThatLiesAboutItsSizeStopsAtTheDeclaredSize(t *testing.T) {
	data := []byte(strings.Repeat("a,b\n", 100))
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateRaw(&zip.FileHeader{
		Name: "companies.csv", Method: zip.Store,
		CRC32: crc32.ChecksumIEEE(data), CompressedSize64: uint64(len(data)), UncompressedSize64: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := OpenBundle(buf.Bytes(), 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := b.Open("companies.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if _, err := io.ReadAll(rc); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("reading a lying entry: err = %v, want ErrBundleRefused", err)
	}
}

func TestTheTemplateFileNameOfEveryTypeIsDetectedAsThatType(t *testing.T) {
	for _, object := range ImportOrder {
		if got := DetectObject("export/" + FileNameFor(object)); got != object {
			t.Errorf("FileNameFor(%s) = %q is detected as %q", object, FileNameFor(object), got)
		}
	}
}

func TestIsZipReadsTheSignatureNotTheName(t *testing.T) {
	if !IsZip(zipOf(t, map[string]string{"a.csv": "a\n"})) || IsZip([]byte("PK,name\n")) {
		t.Fatal("IsZip must answer from the local-file signature PK\\x03\\x04")
	}
}
