// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// ImportOrder is the order a bundle's types land in. A contact names its company,
// a lead may name a contact, and a deal names both, so the record named lands first.
var ImportOrder = []string{ObjectCompany, ObjectContact, ObjectLead, ObjectDeal}

// ZipPart names one file of a stored ZIP. Budget is the upload cap the upload
// passed to OpenBundle, so a walk accepts what the upload accepted.
type ZipPart struct {
	Key, Entry string
	Budget     int64
}

// openZipEntry reads the stored ZIP under the upload's own limits. It returns
// the entry through Bundle.Open, never the raw zip file. An entry that lies
// about its size then stops at the size it declared.
func openZipEntry(ctx context.Context, blobs blobstore.Store, part ZipPart) (io.ReadCloser, error) {
	blob, _, err := blobs.Get(ctx, part.Key)
	if err != nil {
		return nil, fmt.Errorf("import source %q: %w", part.Key, err)
	}
	body, err := io.ReadAll(io.LimitReader(blob, part.Budget+1))
	if cerr := blob.Close(); cerr != nil {
		slog.WarnContext(ctx, "closing the import source", "key", part.Key, "err", cerr)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnreadable, err)
	}
	if int64(len(body)) > part.Budget {
		return nil, fmt.Errorf("%w: the stored ZIP is larger than the upload limit", ErrBundleRefused)
	}
	bundle, err := OpenBundle(body, part.Budget)
	if err != nil {
		return nil, err
	}
	return bundle.Open(part.Entry)
}

// BundleSource is a Source of many parts, one per type, walked in ImportOrder.
// The engine checkpoints one offset over the objects in turn. The order must
// not depend on how the request listed the parts.
type BundleSource struct {
	parts []*CSVSource
}

// NewBundleSource orders the parts by ImportOrder.
//
// One file per type is refused at four doors. Each sees a different input
// (ZIP entries, sources, a request, a form), so no one function serves them.
func NewBundleSource(parts ...*CSVSource) (*BundleSource, error) {
	byObject := map[string]*CSVSource{}
	for _, p := range parts {
		if !slices.Contains(ImportOrder, p.object) {
			return nil, fmt.Errorf("%w: %q is not an importable type", ErrBundleRefused, p.object)
		}
		if _, taken := byObject[p.object]; taken {
			return nil, fmt.Errorf("%w: two files are both %s", ErrBundleRefused, p.object)
		}
		byObject[p.object] = p
	}
	ordered := make([]*CSVSource, 0, len(byObject))
	for _, object := range ImportOrder {
		if p, ok := byObject[object]; ok {
			ordered = append(ordered, p)
		}
	}
	return &BundleSource{parts: ordered}, nil
}

// Objects lists the types the bundle holds, in import order.
func (b *BundleSource) Objects() []string {
	out := make([]string, 0, len(b.parts))
	for _, p := range b.parts {
		out = append(out, p.object)
	}
	return out
}

// Counts merges the row count of every part.
func (b *BundleSource) Counts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	for _, p := range b.parts {
		counts, err := p.Counts(ctx)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, counts)
	}
	return out, nil
}

// Rows pages the part that holds object.
func (b *BundleSource) Rows(ctx context.Context, object string, offset, limit int) ([]Row, error) {
	p, ok := b.Part(object)
	if !ok {
		return nil, fmt.Errorf("%w: this bundle holds no %q file", ErrObjectNotInSource, object)
	}
	return p.Rows(ctx, object, offset, limit)
}

// Associations concatenates the edges each part names.
func (b *BundleSource) Associations(ctx context.Context) ([]Assoc, error) {
	var out []Assoc
	for _, p := range b.parts {
		edges, err := p.Associations(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, edges...)
	}
	return out, nil
}

// Part is the source of one type's file.
func (b *BundleSource) Part(object string) (*CSVSource, bool) {
	for _, p := range b.parts {
		if p.object == object {
			return p, true
		}
	}
	return nil, false
}

// Skipped reports the rows one part could not deliver; empty for a type the
// bundle does not hold.
func (b *BundleSource) Skipped(object string) []SkippedLine {
	if p, ok := b.Part(object); ok {
		return p.Skipped()
	}
	return nil
}
