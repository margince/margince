// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"context"

	"github.com/gradionhq/margince/backend/internal/platform/config"
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

// FromEnv builds a Store from the MARGINCE_BLOBSTORE_* configuration. Secrets
// come from the environment, never CLI flags (which leak into the process
// table). It reports configured=false with a nil Store when neither an endpoint
// nor a path is set, so a deployment without object storage boots normally —
// the attachment endpoints answer 501 rather than the process failing to start.
//
// Two providers can be configured, and ENDPOINT wins when both are. An operator
// who set both has named two different stores for the same bytes, and the
// endpoint is the one that may already hold the objects this installation has
// written; preferring an empty local directory would present a working
// installation whose attachments had all vanished.
//
// The lookup is a parameter (OPS-CFG-2): the composition root supplies the
// environment, and a test supplies a map without mutating process state.
//
//nolint:ireturn // the seam has three providers behind one Store; returning the interface is the design.
func FromEnv(ctx context.Context, env config.Lookup) (store Store, configured bool, err error) {
	endpoint := env(EnvEndpoint)
	if endpoint == "" {
		// The filesystem provider: local disk, for an installation with no
		// object storage service to point at. See NewFilesystem.
		if path := env(EnvPath); path != "" {
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
		AccessKey: env(EnvAccessKey),
		SecretKey: env(EnvSecretKey),
		Bucket:    env(EnvBucket),
		Region:    env(EnvRegion),
		UseSSL:    env(EnvUseSSL) == "true",
	})
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// ConfigItems declares this package's surface for the composition root that
// wires it. Endpoint or Path is what turns the seam on: with both unset the
// other five are never read, which is why none of them is Required — an
// installation without object storage is a supported deployment, not a
// misconfigured one.
func ConfigItems() []config.Item {
	worker := []string{config.RoleAPI, config.RoleWorker}
	return []config.Item{
		{
			Name: EnvEndpoint, Kind: config.KindString, Roles: worker,
			Doc: "S3/MinIO endpoint for attachment bytes; unset disables object storage and the attachment endpoints answer 501",
		},
		{
			Name: EnvAccessKey, Kind: config.KindString, Secret: true, Roles: worker,
			Doc: "access key for the object store",
		},
		{
			Name: EnvSecretKey, Kind: config.KindString, Secret: true, Roles: worker,
			Doc: "secret key for the object store",
		},
		{
			Name: EnvBucket, Kind: config.KindString, Roles: worker,
			Doc: "bucket attachments are written to",
		},
		{
			Name: EnvRegion, Kind: config.KindString, Roles: worker,
			Doc: "region the bucket lives in",
		},
		{
			Name: EnvUseSSL, Kind: config.KindBool, Default: "false", Roles: worker,
			Doc: "reach the object store over TLS; the literal \"true\" enables it",
		},
		{
			Name: EnvPath, Kind: config.KindString, Roles: worker,
			Doc: "directory attachment bytes are written to, for an installation with no object storage service; ignored when an endpoint is set",
		},
	}
}
