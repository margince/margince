// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemmail

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

// The bytes are bounded whatever the row recorded about their size.
func TestReadRefusesMoreBytesThanTheBoundWhateverTheRowSays(t *testing.T) {
	ctx := context.Background()
	store := blobstore.NewMemory()
	files := Files{Blob: store}
	for name, size := range map[string]int{"at the bound": extension.MaxInboundFileBytes, "past it": extension.MaxInboundFileBytes + 1} {
		if err := store.Put(ctx, name, bytes.NewReader(make([]byte, size)), int64(size), "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := files.Read(ctx, extension.MailAttachment{}, "at the bound"); err != nil || len(got.Content) != extension.MaxInboundFileBytes {
		t.Errorf("a file at the bound: %d bytes, %v; want all of it", len(got.Content), err)
	}
	if _, err := files.Read(ctx, extension.MailAttachment{}, "past it"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a file past the bound: err = %v, want ErrInvalid", err)
	}
}
