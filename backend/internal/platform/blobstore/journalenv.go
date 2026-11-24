// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"context"

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

var journalStore = storeSettings{
	endpoint: config.Define(config.Spec[string]{
		Key: "erasure_journal_endpoint", Env: EnvJournalEndpoint, Type: config.String, Roles: journalRoles,
		Doc: "Endpoint of a separate object store for the erasure journal, so a restore of the main store never rolls it back; it wins over `MARGINCE_ERASURE_JOURNAL_PATH`, and with both unset the journal stays in the main store.",
	}),
	accessKey: config.Define(config.Spec[string]{
		Key: "erasure_journal_access_key", Env: EnvJournalAccessKey, Type: config.String, Secret: true, Roles: journalRoles,
		Doc: "Access key for the erasure journal's object store.",
	}),
	secretKey: config.Define(config.Spec[string]{
		Key: "erasure_journal_secret_key", Env: EnvJournalSecretKey, Type: config.String, Secret: true, Roles: journalRoles,
		Doc: "Secret key for the erasure journal's object store.",
	}),
	bucket: config.Define(config.Spec[string]{
		Key: "erasure_journal_bucket", Env: EnvJournalBucket, Type: config.String, Roles: journalRoles,
		Doc: "Bucket the erasure journal is written to.",
	}),
	region: config.Define(config.Spec[string]{
		Key: "erasure_journal_region", Env: EnvJournalRegion, Type: config.String, Roles: journalRoles,
		Doc: "Region the erasure journal's bucket lives in.",
	}),
	useSSL: config.Define(config.Spec[bool]{
		Key: "erasure_journal_use_ssl", Env: EnvJournalUseSSL, Type: config.Bool, Default: config.Some(false), Roles: journalRoles,
		Doc: "Reach the erasure journal's object store over TLS.",
	}),
	path: config.Define(config.Spec[string]{
		Key: "erasure_journal_path", Env: EnvJournalPath, Type: config.String, Roles: journalRoles,
		Doc: "Directory the erasure journal is written to when there is no object storage service for it; ignored when `MARGINCE_ERASURE_JOURNAL_ENDPOINT` is set.",
	}),
}

func journalSettings() []config.Any {
	s := journalStore
	return []config.Any{s.endpoint, s.accessKey, s.secretKey, s.bucket, s.region, s.useSSL, s.path}
}

// JournalFromConfig builds the store the erasure journal lives in. With neither
// a journal endpoint nor a journal path set it reports configured=false, and
// the journal stays in the main store the caller already holds.
//
//nolint:ireturn // same seam as FromConfig.
func JournalFromConfig(ctx context.Context, c *config.Config) (store Store, configured bool, err error) {
	return fromSettings(ctx, c, journalStore)
}
