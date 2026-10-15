// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The variable names this package answers to. Declared next to the code that
// reads them so the surface can be enumerated from the package rather than
// maintained as a list somewhere else — which is what let six production
// variables go undocumented across the tree.
// #nosec G101 -- these are the NAMES of variables, not their values; the whole point of naming them here is that the credential itself never appears in Go
const (
	EnvEndpoint  = "MARGINCE_BLOBSTORE_ENDPOINT"
	EnvAccessKey = "MARGINCE_BLOBSTORE_ACCESS_KEY"
	EnvSecretKey = "MARGINCE_BLOBSTORE_SECRET_KEY"
	EnvBucket    = "MARGINCE_BLOBSTORE_BUCKET"
	EnvRegion    = "MARGINCE_BLOBSTORE_REGION"
	EnvUseSSL    = "MARGINCE_BLOBSTORE_USE_SSL"
	EnvPath      = "MARGINCE_BLOBSTORE_PATH"
)

// FromConfig builds a Store from the blobstore settings. The secrets have no
// flag, since a flag leaks into the process table. It reports configured=false with a nil Store when neither an endpoint
// nor a path is set, so a deployment without object storage boots normally —
// the attachment endpoints answer 501 rather than the process failing to start.
//
// Two providers can be configured, and ENDPOINT wins when both are. An operator
// who set both has named two different stores for the same bytes, and the
// endpoint is the one that may already hold the objects this installation has
// written; preferring an empty local directory would present a working
// installation whose attachments had all vanished.
//
//nolint:ireturn // the seam has three providers behind one Store; returning the interface is the design.
func FromConfig(ctx context.Context, c *config.Config) (store Store, configured bool, err error) {
	endpoint := config.Val(c, endpointSetting)
	if endpoint == "" {
		// The filesystem provider: local disk, for an installation with no
		// object storage service to point at. See NewFilesystem.
		if path := config.Val(c, pathSetting); path != "" {
			s, ferr := NewFilesystem(path)
			if ferr != nil {
				return nil, false, ferr
			}
			return s, true, nil
		}
		return nil, false, nil
	}
	s, err := New(ctx, Config{
		Endpoint:  endpoint,
		AccessKey: config.Val(c, accessKeySetting),
		SecretKey: config.Val(c, secretKeySetting),
		Bucket:    config.Val(c, bucketSetting),
		Region:    config.Val(c, regionSetting),
		UseSSL:    config.Val(c, useSSLSetting),
	})
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// The settings that turn object storage on. Endpoint or Path enables it: with
// both unset the other five are never read, which is why none is required.
var (
	storeRoles = []config.Role{config.RoleAPI, config.RoleWorker}

	endpointSetting = config.Define(config.Spec[string]{
		Key: "blobstore_endpoint", Env: EnvEndpoint, Type: config.String, Roles: storeRoles,
		Doc: "S3/MinIO endpoint for attachment bytes; it wins over MARGINCE_BLOBSTORE_PATH, and with both unset the attachment endpoints answer 501.",
	})
	accessKeySetting = config.Define(config.Spec[string]{
		Key: "blobstore_access_key", Env: EnvAccessKey, Type: config.String, Secret: true, Roles: storeRoles,
		Doc: "Access key for the object store.",
	})
	secretKeySetting = config.Define(config.Spec[string]{
		Key: "blobstore_secret_key", Env: EnvSecretKey, Type: config.String, Secret: true, Roles: storeRoles,
		Doc: "Secret key for the object store.",
	})
	bucketSetting = config.Define(config.Spec[string]{
		Key: "blobstore_bucket", Env: EnvBucket, Type: config.String, Roles: storeRoles,
		Doc: "Bucket attachments are written to.",
	})
	regionSetting = config.Define(config.Spec[string]{
		Key: "blobstore_region", Env: EnvRegion, Type: config.String, Roles: storeRoles,
		Doc: "Region the bucket lives in.",
	})
	useSSLSetting = config.Define(config.Spec[bool]{
		Key: "blobstore_use_ssl", Env: EnvUseSSL, Type: config.Bool, Default: config.Some(false), Roles: storeRoles,
		Doc: "Reach the object store over TLS.",
	})
	pathSetting = config.Define(config.Spec[string]{
		Key: "blobstore_path", Env: EnvPath, Type: config.String, Roles: storeRoles,
		Doc: "Directory attachment bytes are written to when there is no object storage service; ignored when MARGINCE_BLOBSTORE_ENDPOINT is set.",
	})
)

// Settings are this package's boot settings.
func Settings() []config.Any {
	return []config.Any{endpointSetting, accessKeySetting, secretKeySetting, bucketSetting, regionSetting, useSSLSetting, pathSetting}
}
