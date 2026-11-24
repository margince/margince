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

// FromConfig builds a Store from the blobstore settings. With neither an
// endpoint nor a path set it reports configured=false and a nil Store, and the
// attachment endpoints answer 501. ENDPOINT wins over PATH because it may
// already hold the objects this installation wrote.
//
//nolint:ireturn // the seam has three providers behind one Store; returning the interface is the design.
func FromConfig(ctx context.Context, c *config.Config) (store Store, configured bool, err error) {
	return fromSettings(ctx, c, attachmentStore)
}

// storeSettings is one set of the seven settings that name an object store. The
// attachment store and the erasure journal store each have one, and
// fromSettings builds either, so a second client is never written.
type storeSettings struct {
	endpoint, accessKey, secretKey, bucket, region, path *config.Setting[string]
	useSSL                                               *config.Setting[bool]
}

//nolint:ireturn // same seam as FromConfig.
func fromSettings(ctx context.Context, c *config.Config, set storeSettings) (store Store, configured bool, err error) {
	endpoint := config.Val(c, set.endpoint)
	if endpoint == "" {
		// The filesystem provider: local disk, for an installation with no
		// object storage service to point at. See NewFilesystem.
		if path := config.Val(c, set.path); path != "" {
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
		AccessKey: config.Val(c, set.accessKey),
		SecretKey: config.Val(c, set.secretKey),
		Bucket:    config.Val(c, set.bucket),
		Region:    config.Val(c, set.region),
		UseSSL:    config.Val(c, set.useSSL),
	})
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// The settings that turn object storage on. Endpoint or Path enables it: with
// both unset the other five are never read, which is why none is required.
var (
	storeRoles = []config.Role{config.RoleAPI, config.RoleWorker, config.RoleMigrate}

	endpointSetting = config.Define(config.Spec[string]{
		Key: "blobstore_endpoint", Env: EnvEndpoint, Type: config.String, Roles: storeRoles,
		Doc: "Endpoint of an object store such as MinIO for attachment bytes; it wins over MARGINCE_BLOBSTORE_PATH, and with both unset the attachment endpoints answer 501.",
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
		Doc: "Bucket attachments are written to. Its credentials need `s3:PutObject`, `s3:GetObject`, `s3:DeleteObject` and `s3:ListBucket`. Erasure also needs `s3:ListBucketVersions` and `s3:DeleteObjectVersion`, so no version of a deleted object stays behind.",
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

var attachmentStore = storeSettings{
	endpoint: endpointSetting, accessKey: accessKeySetting, secretKey: secretKeySetting,
	bucket: bucketSetting, region: regionSetting, useSSL: useSSLSetting, path: pathSetting,
}

// Settings are this package's boot settings.
func Settings() []config.Any {
	return append(
		[]config.Any{endpointSetting, accessKeySetting, secretKeySetting, bucketSetting, regionSetting, useSSLSetting, pathSetting},
		journalSettings()...)
}
