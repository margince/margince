// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// A bundle is a ZIP of CSV files, one per record type. The entry count and the
// declared sizes are checked before any entry is read. A read then stops at the
// size its entry declared, and never past the bundle's limit.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// ErrBundleRefused is a ZIP that is unreadable, too large, protected, or
// ambiguous about which file holds which type.
var ErrBundleRefused = errors.New("import bundle refused")

// MaxBundleEntries bounds the files in one bundle.
const MaxBundleEntries = 20

// declaredSizeFactor is how many times the upload cap the declared sizes may
// add up to. A ZIP is smaller than the CSV it expands to.
const declaredSizeFactor = 10

// Why an entry of the bundle is listed but not imported.
const (
	IgnoredUnknownName      = "unknown_name"
	IgnoredNotYetImportable = "not_yet_importable"
)

var zipSignature = []byte("PK\x03\x04")

// bundleFileObjects maps a file's base name, lower-cased and without ".csv", to
// the object it imports. Both the plural the template writes and the singular
// someone types are accepted.
var bundleFileObjects = map[string]string{
	"companies": ObjectCompany, "company": ObjectCompany,
	"contacts": ObjectContact, "contact": ObjectContact,
	"leads": ObjectLead, "lead": ObjectLead,
	"deals": ObjectDeal, "deal": ObjectDeal,
}

// notYetImportable names files of a type a later writer takes; they are listed,
// not refused, so a template ZIP from a newer installation still imports.
var notYetImportable = map[string]bool{"activities": true, "activity": true, "notes": true}

// templateFileStems is the plural each type's template file carries; it must
// be a key of bundleFileObjects, which the detection test holds.
var templateFileStems = map[string]string{
	ObjectCompany: "companies", ObjectContact: "contacts", ObjectLead: "leads", ObjectDeal: "deals",
}

// BundlePart is one CSV of the bundle matched to the type it imports.
type BundlePart struct {
	Entry  string
	Object string
	Size   uint64
}

// IgnoredEntry is a file the bundle holds that no type takes.
type IgnoredEntry struct {
	Entry  string
	Reason string
}

// Bundle is an opened ZIP whose files are matched to types.
type Bundle struct {
	Parts   []BundlePart
	Ignored []IgnoredEntry
	zr      *zip.Reader
	limit   uint64
}

// IsZip tells a ZIP from a CSV by the local-file signature, not the name.
func IsZip(body []byte) bool {
	return bytes.HasPrefix(body, zipSignature)
}

// DetectObject is the type a file name imports, or empty when none does.
func DetectObject(fileName string) string {
	return bundleFileObjects[stemOf(fileName)]
}

// FileNameFor is the file name the template writes for a type, or empty for a
// type with no file.
func FileNameFor(object string) string {
	stem, ok := templateFileStems[object]
	if !ok {
		return ""
	}
	return stem + ".csv"
}

func stemOf(fileName string) string {
	return strings.TrimSuffix(strings.ToLower(path.Base(fileName)), ".csv")
}

// silentlySkipped is what a ZIP tool adds and nobody chose: macOS resource
// forks, Finder state, folders, and the template's own README.
func silentlySkipped(f *zip.File) bool {
	base := path.Base(f.Name)
	return f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") ||
		strings.HasPrefix(base, ".") || strings.EqualFold(base, "README.txt")
}

// OpenBundle checks a ZIP against the limits and matches its files to types.
func OpenBundle(body []byte, uploadCap int64) (Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return Bundle{}, fmt.Errorf("%w: not a readable ZIP: %v", ErrBundleRefused, err)
	}
	limit := uint64(declaredSizeFactor) * uint64(max(uploadCap, 0))
	var entries []*zip.File
	var declared uint64
	for _, f := range zr.File {
		if silentlySkipped(f) {
			continue
		}
		entries = append(entries, f)
		// The sizes come from the ZIP's own headers. Comparing before adding
		// keeps two huge sizes from wrapping the sum back under the limit.
		if f.UncompressedSize64 > limit-declared {
			return Bundle{}, fmt.Errorf("%w: the files add up to more than %d MB, the most that is read",
				ErrBundleRefused, limit>>20)
		}
		declared += f.UncompressedSize64
	}
	if len(entries) > MaxBundleEntries {
		return Bundle{}, fmt.Errorf("%w: %d files, at most %d are read", ErrBundleRefused, len(entries), MaxBundleEntries)
	}
	b := Bundle{zr: zr, limit: limit}
	byObject := map[string]string{}
	for _, f := range entries {
		if f.Flags&0x1 != 0 {
			return Bundle{}, fmt.Errorf("%w: %s is password-protected", ErrBundleRefused, f.Name)
		}
		stem := stemOf(f.Name)
		object := bundleFileObjects[stem]
		switch {
		case object != "":
			if first, dup := byObject[object]; dup {
				return Bundle{}, fmt.Errorf("%w: %s and %s both hold %s; keep one", ErrBundleRefused, first, f.Name, object)
			}
			byObject[object] = f.Name
			b.Parts = append(b.Parts, BundlePart{Entry: f.Name, Object: object, Size: f.UncompressedSize64})
		case notYetImportable[stem]:
			b.Ignored = append(b.Ignored, IgnoredEntry{Entry: f.Name, Reason: IgnoredNotYetImportable})
		default:
			b.Ignored = append(b.Ignored, IgnoredEntry{Entry: f.Name, Reason: IgnoredUnknownName})
		}
	}
	return b, nil
}

// Open reads one entry, stopping at the size it declared or the bundle's limit,
// whichever is smaller.
func (b Bundle) Open(entry string) (io.ReadCloser, error) {
	for _, f := range b.zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrBundleRefused, entry, err)
		}
		return &sizeGuard{rc: rc, entry: entry, left: min(f.UncompressedSize64, b.limit)}, nil
	}
	return nil, fmt.Errorf("%w: %s is not in the bundle", ErrBundleRefused, entry)
}

// sizeGuard refuses an entry that yields more bytes than its header declared,
// the way a decompression bomb hides behind a small declared size.
type sizeGuard struct {
	rc    io.ReadCloser
	entry string
	left  uint64
}

func (g *sizeGuard) Read(p []byte) (int, error) {
	n, err := g.rc.Read(p)
	var read uint64
	if n > 0 {
		read = uint64(n)
	}
	if read > g.left {
		g.left = 0
		return 0, fmt.Errorf("%w: %s is larger than it says", ErrBundleRefused, g.entry)
	}
	g.left -= read
	if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) {
		return n, fmt.Errorf("%w: %s is damaged", ErrBundleRefused, g.entry)
	}
	return n, err
}

func (g *sizeGuard) Close() error { return g.rc.Close() }
