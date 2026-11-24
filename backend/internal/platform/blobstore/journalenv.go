// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The variable names of the erasure journal's own object store. Same seven
// settings as the attachment store, under another prefix.
// #nosec G101 -- these are the NAMES of variables, not their values
const (
	EnvJournalEndpoint  = "MARGINCE_ERASURE_JOURNAL_ENDPOINT"
	EnvJournalAccessKey = "MARGINCE_ERASURE_JOURNAL_ACCESS_KEY"
	EnvJournalSecretKey = "MARGINCE_ERASURE_JOURNAL_SECRET_KEY"
	EnvJournalBucket    = "MARGINCE_ERASURE_JOURNAL_BUCKET"
	EnvJournalRegion    = "MARGINCE_ERASURE_JOURNAL_REGION"
	EnvJournalUseSSL    = "MARGINCE_ERASURE_JOURNAL_USE_SSL"
	EnvJournalPath      = "MARGINCE_ERASURE_JOURNAL_PATH"
)

var journalRoles = []config.Role{config.RoleWorker}

var journalEndpointSetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_endpoint", Env: EnvJournalEndpoint, Type: config.String, Roles: journalRoles,
	Doc: "Endpoint of a separate object store for the erasure journal, so a restore of the main store never rolls it back; it wins over `MARGINCE_ERASURE_JOURNAL_PATH`, and with both unset the journal stays in the main store.",
})

var journalAccessKeySetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_access_key", Env: EnvJournalAccessKey, Type: config.String, Secret: true, Roles: journalRoles,
	Doc: "Access key for the erasure journal's object store.",
})

var journalSecretKeySetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_secret_key", Env: EnvJournalSecretKey, Type: config.String, Secret: true, Roles: journalRoles,
	Doc: "Secret key for the erasure journal's object store.",
})

var journalBucketSetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_bucket", Env: EnvJournalBucket, Type: config.String, Roles: journalRoles,
	Doc: "Bucket the erasure journal is written to.",
})

var journalRegionSetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_region", Env: EnvJournalRegion, Type: config.String, Roles: journalRoles,
	Doc: "Region the erasure journal's bucket lives in.",
})

var journalUseSSLSetting = config.Define(config.Spec[bool]{
	Key: "erasure_journal_use_ssl", Env: EnvJournalUseSSL, Type: config.Bool, Default: config.Some(false), Roles: journalRoles,
	Doc: "Reach the erasure journal's object store over TLS.",
})

var journalPathSetting = config.Define(config.Spec[string]{
	Key: "erasure_journal_path", Env: EnvJournalPath, Type: config.String, Roles: journalRoles,
	Doc: "Directory the erasure journal is written to when there is no object storage service for it; ignored when `MARGINCE_ERASURE_JOURNAL_ENDPOINT` is set.",
})

var journalStore = storeSettings{
	endpoint: journalEndpointSetting, accessKey: journalAccessKeySetting, secretKey: journalSecretKeySetting, bucket: journalBucketSetting, region: journalRegionSetting, useSSL: journalUseSSLSetting, path: journalPathSetting,
}

func journalSettings() []config.Any {
	s := journalStore
	return []config.Any{s.endpoint, s.accessKey, s.secretKey, s.bucket, s.region, s.useSSL, s.path}
}

// JournalFromConfig builds the store the erasure journal lives in. With no
// journal setting at all it reports configured=false, and the journal stays in
// the main store the caller already holds.
//
// Any journal setting means a separate store was asked for, so an incomplete
// set is an error naming what is missing. A set that names the main store's
// own place is refused, because a restore would roll the journal back.
//
//nolint:ireturn // same seam as FromConfig.
func JournalFromConfig(ctx context.Context, c *config.Config) (store Store, configured bool, err error) {
	set := journalStore
	endpoint, path := config.Val(c, set.endpoint), config.Val(c, set.path)
	asked := endpoint != "" || path != "" || config.Val(c, set.useSSL)
	for _, v := range []*config.Setting[string]{set.accessKey, set.secretKey, set.bucket, set.region} {
		asked = asked || config.Val(c, v) != ""
	}
	if !asked {
		return nil, false, nil
	}
	if missing := journalMissing(c, set, endpoint, path); len(missing) > 0 {
		return nil, false, fmt.Errorf(
			"an erasure journal store is partly set; also set %s, or unset every MARGINCE_ERASURE_JOURNAL_* setting to keep the journal in the main store",
			strings.Join(missing, ", "))
	}
	if sameLocation(c, set, endpoint, path) {
		return nil, false, errors.New(
			"the erasure journal store is the same place as the main object store, so a restore would roll the journal back; " +
				"point MARGINCE_ERASURE_JOURNAL_* at a different store or unset them")
	}
	return fromSettings(ctx, c, set)
}

func journalMissing(c *config.Config, set storeSettings, endpoint, path string) []string {
	if endpoint == "" && path == "" {
		return []string{EnvJournalEndpoint + " (with " + EnvJournalBucket + " and " + EnvJournalRegion + ") or " + EnvJournalPath}
	}
	var missing []string
	if endpoint != "" {
		if config.Val(c, set.bucket) == "" {
			missing = append(missing, EnvJournalBucket)
		}
		if config.Val(c, set.region) == "" {
			missing = append(missing, EnvJournalRegion)
		}
	}
	return missing
}

// sameLocation reports whether the journal store names the place the main
// store lives. The main store's endpoint wins over its path, as in FromConfig.
func sameLocation(c *config.Config, set storeSettings, endpoint, path string) bool {
	mainEndpoint := config.Val(c, attachmentStore.endpoint)
	switch {
	case endpoint != "" && mainEndpoint != "":
		return endpoint == mainEndpoint && config.Val(c, set.bucket) == config.Val(c, attachmentStore.bucket)
	case endpoint == "" && mainEndpoint == "":
		mainPath := config.Val(c, attachmentStore.path)
		return path != "" && mainPath != "" && filepath.Clean(path) == filepath.Clean(mainPath)
	default:
		return false
	}
}

// ResolveJournal answers the store the suppression journal lives in: the
// separate one when any journal setting is set, else main. separate says which.
// The export job and the replay command both call it, so the writer and the
// reader never name different places.
//
//nolint:ireturn // the store seam is an interface by design.
func ResolveJournal(ctx context.Context, c *config.Config, main Store) (store Store, separate bool, err error) {
	store, separate, err = JournalFromConfig(ctx, c)
	if err != nil {
		return nil, false, fmt.Errorf("erasure journal store: %w", err)
	}
	if !separate {
		return main, false, nil
	}
	return store, true, nil
}
