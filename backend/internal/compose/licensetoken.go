// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The licence token. MARGINCE_LICENSE is also the name the bundled validation
// module reads, so a container that exports it needs no license: block.
var (
	LicenseToken = config.Define(config.Spec[string]{
		Key: "license.token", Env: "MARGINCE_LICENSE", Type: config.String, Secret: true, Roles: bothRoles,
		Doc: "Licence token; unset with no license.token_file runs unlicensed, which production refuses.",
	})
	LicenseTokenFile = config.Define(config.Spec[string]{
		Key: "license.token_file", Env: "MARGINCE_LICENSE_TOKEN_FILE", Type: config.String, Roles: bothRoles,
		Doc: "Path to a file holding the licence token, read again at each licence check; license.token wins over it.",
	})
)

// bothRoles are the two serving programs.
var bothRoles = []config.Role{config.RoleAPI, config.RoleWorker}

// licenseTokenOriginNone is the boot line's answer when nothing names a token.
const licenseTokenOriginNone = "none"

// licenseTokenLimit bounds a token file. A licence is a JWT of a few
// kilobytes, and everything downstream copies it whole. So a path pointed at a
// log or an image fails as the mistake it is.
const licenseTokenLimit = 64 << 10

// declaredLicenseToken reads the token the deployment declares, or "" for an
// unlicensed installation. A named source that yields nothing is an error. In
// production, "no licence" and "an empty file" would otherwise refuse the boot
// with the wrong remedy.
//
// A source rather than a value because token_file is read again at each
// check, so a licence renewed there takes effect without a restart. The
// license.token value is fixed at boot.
func declaredLicenseToken(c *config.Config) func() (string, error) {
	token, origin := config.ValWithSource(c, LicenseToken)
	path, pathOrigin := config.ValWithSource(c, LicenseTokenFile)
	return func() (string, error) {
		if origin.Source != config.SourceUnset {
			if strings.TrimSpace(token) == "" {
				return "", fmt.Errorf("compose: %s holds no licence token — remove it to run unlicensed, or fill it", origin)
			}
			return strings.TrimSpace(token), nil
		}
		if path == "" {
			return "", nil
		}
		raw, err := readDeclaredFile(pathOrigin.Name, path, licenseTokenLimit)
		if err != nil {
			return "", err
		}
		if raw == "" {
			return "", fmt.Errorf("compose: %s %s is empty — remove the setting to run unlicensed, "+
				"or write the license token into the file", pathOrigin.Name, path)
		}
		return raw, nil
	}
}

// licenseTokenDeclaredAt names the setting the token comes from, by the name
// the operator wrote: a variable or a margince.yaml key.
func licenseTokenDeclaredAt(c *config.Config) string {
	if _, origin := config.ValWithSource(c, LicenseToken); origin.Source != config.SourceUnset {
		return origin.Name
	}
	if path, origin := config.ValWithSource(c, LicenseTokenFile); path != "" {
		return origin.Name
	}
	return licenseTokenOriginNone
}

// readDeclaredFile reads a credential a setting names by path, trimmed, and
// refuses a file too large to be one before reading it.
func readDeclaredFile(setting, path string, limit int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("compose: reading %s: %w", setting, err)
	}
	if info.Size() > limit {
		return "", fmt.Errorf("compose: %s %s is %d bytes, more than the credential it names (limit %d) — "+
			"check the path points at the credential and not at something else", setting, path, info.Size(), limit)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- the operator's own credential path; reading it is the function's purpose
	if err != nil {
		return "", fmt.Errorf("compose: reading %s: %w", setting, err)
	}
	return strings.TrimSpace(string(raw)), nil
}
