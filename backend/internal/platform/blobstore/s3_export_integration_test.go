// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package blobstore

// WithListPageSize lowers the version-listing page size of an S3 store, so a
// test can cross a page boundary with a handful of objects.
func WithListPageSize(store Store, n int) {
	if s, ok := store.(*s3Store); ok {
		s.listPageSize = n
	}
}
