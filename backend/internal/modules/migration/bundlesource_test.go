// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
)

const bundleKey = "ws/import/bundle-1"

func storeBundle(t *testing.T, body []byte) blobstore.Store {
	t.Helper()
	bs := blobstore.NewMemory()
	if err := bs.Put(context.Background(), bundleKey, strings.NewReader(string(body)), int64(len(body)), "application/zip"); err != nil {
		t.Fatal(err)
	}
	return bs
}

func seedBundle(t *testing.T, files map[string]string) blobstore.Store {
	t.Helper()
	return storeBundle(t, zipOf(t, files))
}

func twoPartBundle(t *testing.T) *BundleSource {
	t.Helper()
	bs := seedBundle(t, map[string]string{
		"contacts.csv":  "ref,email\nP1,jo@acme.test\nP2,lan@beta.test\n",
		"companies.csv": "ref,name\nC1,Acme\n",
	})
	part := func(entry, object string, mapping map[string]string) *CSVSource {
		return NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: entry, Budget: 10 << 20}, object, mapping, "ref")
	}
	src, err := NewBundleSource(
		part("contacts.csv", ObjectContact, map[string]string{"ref": "ref", "email": "email"}),
		part("companies.csv", ObjectCompany, map[string]string{"ref": "ref", "name": "display_name"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestABundleWalksItsPartsInImportOrder(t *testing.T) {
	src := twoPartBundle(t)
	if got := src.Objects(); !slices.Equal(got, []string{ObjectCompany, ObjectContact}) {
		t.Fatalf("objects = %v, want companies before contacts whatever order the request named them", got)
	}
	runs, w := newFakeRuns(), newFakeWriters()
	if _, err := (&Engine{runs: runs, w: w}).Run(context.Background(), RunID{}, src); err != nil {
		t.Fatal(err)
	}
	want := []string{"company/C1", "contact/P1", "contact/P2"}
	if !slices.Equal(w.ensured, want) {
		t.Fatalf("landed %v, want %v", w.ensured, want)
	}
}

func TestABundleResumesInsideItsSecondPart(t *testing.T) {
	src := twoPartBundle(t)
	runs, w := newFakeRuns(), newFakeWriters()
	w.failAt = 3 // the second contact
	e := &Engine{runs: runs, w: w}
	if _, err := e.Run(context.Background(), RunID{}, src); err == nil {
		t.Fatal("want the injected crash")
	}
	runs.run.Status, w.failAt = StatusRunning, 0
	if _, err := e.Run(context.Background(), RunID{}, src); err != nil {
		t.Fatal(err)
	}
	if runs.run.Checkpoint != 3 || len(w.ensured) != 3 {
		t.Fatalf("checkpoint %d, landed %v; want 3 rows landed once each", runs.run.Checkpoint, w.ensured)
	}
}

func TestTwoPartsForOneObjectAreRefused(t *testing.T) {
	bs := seedBundle(t, map[string]string{"a.csv": "ref\nx\n"})
	one := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "a.csv", Budget: 1 << 20}, ObjectLead, map[string]string{"ref": "ref"}, "ref")
	if _, err := NewBundleSource(one, one); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v; two parts for one object must be refused: Rows(object) would not know which file to read", err)
	}
}

func TestAPartOfAnObjectOutsideTheImportOrderIsRefused(t *testing.T) {
	bs := seedBundle(t, map[string]string{"a.csv": "ref\nx\n"})
	odd := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "a.csv", Budget: 1 << 20}, "deal", map[string]string{"ref": "ref"}, "ref")
	if _, err := NewBundleSource(odd); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v, want ErrBundleRefused", err)
	}
}

func TestABundleAnswersPartsCountsAndAnObjectItDoesNotHold(t *testing.T) {
	src := twoPartBundle(t)
	counts, err := src.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts[ObjectCompany] != 1 || counts[ObjectContact] != 2 {
		t.Fatalf("counts = %v", counts)
	}
	if _, ok := src.Part(ObjectContact); !ok {
		t.Fatal("the contact part must be reachable")
	}
	if _, ok := src.Part(ObjectLead); ok {
		t.Fatal("a bundle with no lead file has no lead part")
	}
	if _, err := src.Rows(context.Background(), ObjectLead, 0, 10); !errors.Is(err, ErrObjectNotInSource) {
		t.Fatalf("err = %v, want ErrObjectNotInSource", err)
	}
	if got := src.Skipped(ObjectLead); len(got) != 0 {
		t.Fatalf("skipped for an absent part = %v", got)
	}
}

func TestABundleDisclosesTheLinesItsPartSkipped(t *testing.T) {
	bs := seedBundle(t, map[string]string{"companies.csv": "ref,name\n,Nameless\nC1,Acme\n"})
	part := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "companies.csv", Budget: 1 << 20}, ObjectCompany, map[string]string{"ref": "ref", "name": "display_name"}, "ref")
	src, err := NewBundleSource(part)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Rows(context.Background(), ObjectCompany, 0, 10); err != nil {
		t.Fatal(err)
	}
	if got := src.Skipped(ObjectCompany); len(got) != 1 || got[0].Line != 2 {
		t.Fatalf("skipped = %v, want line 2", got)
	}
}

func TestAContactPartKeepsItsOwnAssociations(t *testing.T) {
	bs := seedBundle(t, map[string]string{"contacts.csv": "ref,email,company\nP1,jo@acme.test,Acme\n"})
	part := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "contacts.csv", Budget: 1 << 20}, ObjectContact,
		map[string]string{"ref": "ref", "email": "email", "company": AssocTargetCompanyName}, "ref")
	src, err := NewBundleSource(part)
	if err != nil {
		t.Fatal(err)
	}
	edges, err := src.Associations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %v, want the contact's employment edge", edges)
	}
}

func TestTheWalkStopsAnEntryThatLiesAboutItsSize(t *testing.T) {
	data := []byte("ref,name\n" + strings.Repeat("C1,Acme\n", 100))
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
	bs := storeBundle(t, buf.Bytes())
	part := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "companies.csv", Budget: 10 << 20}, ObjectCompany, map[string]string{"ref": "ref"}, "ref")
	_, err = part.Counts(context.Background())
	if err == nil || !strings.Contains(err.Error(), ErrBundleRefused.Error()) {
		t.Fatalf("err = %v, want the walk to stop at the size the entry declared", err)
	}
}

func TestAStoredZipLargerThanTheUploadCapIsRefused(t *testing.T) {
	bs := seedBundle(t, map[string]string{"companies.csv": "ref,name\nC1,Acme\n"})
	part := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "companies.csv", Budget: 10}, ObjectCompany, map[string]string{"ref": "ref"}, "ref")
	if _, err := part.Counts(context.Background()); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v, want ErrBundleRefused", err)
	}
}

func TestAnEntryTheStoredZipLacksIsRefused(t *testing.T) {
	bs := seedBundle(t, map[string]string{"companies.csv": "ref,name\nC1,Acme\n"})
	part := NewZipPartSource(bs, ZipPart{Key: bundleKey, Entry: "gone.csv", Budget: 1 << 20}, ObjectCompany, map[string]string{"ref": "ref"}, "ref")
	if _, err := part.Header(context.Background()); !errors.Is(err, ErrBundleRefused) {
		t.Fatalf("err = %v, want ErrBundleRefused", err)
	}
}
