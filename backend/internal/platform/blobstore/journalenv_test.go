// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

func TestTheJournalStoreSettingsAreAllOrNothing(t *testing.T) {
	t.Parallel()
	dir, other := t.TempDir(), t.TempDir()
	for _, c := range []struct {
		what      string
		env       []string
		wantErr   string
		wantStore bool
	}{
		{what: "none set keeps the main store", env: nil},
		{what: "a bucket alone is an error naming what is missing", env: []string{blobstore.EnvJournalBucket + "=j"},
			wantErr: blobstore.EnvJournalEndpoint},
		{what: "an endpoint without bucket and region names both", env: []string{blobstore.EnvJournalEndpoint + "=localhost:9000"},
			wantErr: blobstore.EnvJournalBucket + ", " + blobstore.EnvJournalRegion},
		{what: "a path is a complete set", env: []string{blobstore.EnvJournalPath + "=" + dir}, wantStore: true},
		{what: "the same folder as the main store is refused",
			env:     []string{blobstore.EnvJournalPath + "=" + dir, blobstore.EnvPath + "=" + dir},
			wantErr: "same place as the main object store"},
		{what: "another folder than the main store is fine",
			env:       []string{blobstore.EnvJournalPath + "=" + dir, blobstore.EnvPath + "=" + other},
			wantStore: true},
		{what: "the same endpoint and bucket as the main store is refused",
			env: []string{
				blobstore.EnvJournalEndpoint + "=h:9000", blobstore.EnvJournalBucket + "=b", blobstore.EnvJournalRegion + "=r",
				blobstore.EnvEndpoint + "=h:9000", blobstore.EnvBucket + "=b", blobstore.EnvRegion + "=r",
			},
			wantErr: "same place as the main object store"},
	} {
		settings, err := configtest.Load(t, "", c.env, nil, blobstore.Settings()...)
		if err != nil {
			t.Fatalf("%s: loading settings: %v", c.what, err)
		}
		store, configured, err := blobstore.JournalFromConfig(t.Context(), settings)
		switch {
		case c.wantErr != "":
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err = %v, want it to contain %q", c.what, err, c.wantErr)
			}
		case err != nil:
			t.Errorf("%s: unexpected error %v", c.what, err)
		case configured != c.wantStore || (store != nil) != c.wantStore:
			t.Errorf("%s: configured = %v, want %v", c.what, configured, c.wantStore)
		}
	}
}
