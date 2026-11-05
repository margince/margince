// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

func writeLicenseFile(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "license")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func tokenFileYAML(path string) string { return "license:\n  token_file: " + path + "\n" }

// Written the way a secret store or an editor leaves it: a trailing newline.
func TestTheTokenFileIsReadWithoutItsTerminator(t *testing.T) {
	path := writeLicenseFile(t, []byte("a.token.value\n"))
	got, err := declaredLicenseToken(licenseSettings(t, tokenFileYAML(path)))()
	if err != nil || got != "a.token.value" {
		t.Fatalf("token %q, err %v; want the file's contents without its terminator", got, err)
	}
}

// MARGINCE_LICENSE outranks the file, and an empty one falls through to it: a
// container that exports the variable with nothing in it is not unlicensed.
func TestTheVariableOutranksTheFileUnlessItIsEmpty(t *testing.T) {
	path := writeLicenseFile(t, []byte("from.the.file"))
	for _, tc := range []struct{ env, want string }{
		{"MARGINCE_LICENSE= from.the.environment ", "from.the.environment"},
		{"MARGINCE_LICENSE=", "from.the.file"},
	} {
		got, err := declaredLicenseToken(licenseSettings(t, tokenFileYAML(path), tc.env))()
		if err != nil || got != tc.want {
			t.Errorf("%s: token %q, err %v; want %q", tc.env, got, err, tc.want)
		}
	}
}

func TestNothingDeclaredIsAnUnlicensedInstallation(t *testing.T) {
	got, err := declaredLicenseToken(licenseSettings(t, ""))()
	if err != nil || got != "" {
		t.Fatalf("token %q, err %v; want empty", got, err)
	}
}

// A path that does not resolve, an empty file and a file too large to be a
// licence are mistakes. Read as "unlicensed" they would hand the operator a
// workspace that believes it has no entitlement.
func TestATokenFileThatCannotBeALicenseIsRefusedByName(t *testing.T) {
	for name, tc := range map[string]struct {
		path string
		want string
	}{
		"missing":   {filepath.Join(t.TempDir(), "typo"), "license.token_file"},
		"empty":     {writeLicenseFile(t, []byte("\n")), "remove the setting to run unlicensed"},
		"too large": {writeLicenseFile(t, bytes.Repeat([]byte("x"), config.SecretFileLimit+1)), "check the path points at the credential"},
	} {
		if _, err := declaredLicenseToken(licenseSettings(t, tokenFileYAML(tc.path)))(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v; want it to say %q", name, err, tc.want)
		}
	}
}

// The bound refuses what cannot be a licence, not what is merely large.
func TestATokenFileAtTheLimitIsRead(t *testing.T) {
	path := writeLicenseFile(t, bytes.Repeat([]byte("x"), config.SecretFileLimit))
	got, err := declaredLicenseToken(licenseSettings(t, tokenFileYAML(path)))()
	if err != nil || len(got) != config.SecretFileLimit {
		t.Fatalf("read %d bytes, err %v; want %d", len(got), err, config.SecretFileLimit)
	}
}

// The token is a credential, and margince.yaml gets pasted into support
// threads. Nor may it be declared absent: removing a licence is not a
// deployment's to state.
func TestAnInlineOrAbsentTokenInTheFileIsRefused(t *testing.T) {
	for _, value := range []string{"inline-secret", "${none}"} {
		_, err := configtest.Load(t, "license:\n  token: "+value+"\n", nil, nil, LicenseToken, LicenseTokenFile)
		if err == nil || strings.Contains(err.Error(), "inline-secret") {
			t.Errorf("%s: err %v; want a refusal that does not echo the value", value, err)
		}
	}
}

// The boot line names the setting that won, because the environment outranks
// the file an operator reviews.
func TestTheTokenOriginNamesTheSettingThatWins(t *testing.T) {
	file := tokenFileYAML("/etc/margince/license")
	for name, tc := range map[string]struct {
		body, env, want string
	}{
		"nothing configured":              {"", "", "none"},
		"the file reference":              {file, "", "license.token_file"},
		"the environment, over a file":    {file, "MARGINCE_LICENSE=a.token", "MARGINCE_LICENSE"},
		"an empty variable does not win":  {file, "MARGINCE_LICENSE=", "license.token_file"},
		"the reference, over a file path": {"license:\n  token: ${env:T}\n  token_file: /x\n", "T=a.token", "license.token"},
	} {
		if got := licenseTokenDeclaredAt(licenseSettings(t, tc.body, tc.env)); got != tc.want {
			t.Errorf("%s: origin %q, want %q", name, got, tc.want)
		}
	}
}

// A named token source that yields nothing is a mistake, not an unlicensed
// installation; in production the two refuse with different remedies.
func TestAnEmptyTokenReferenceIsAnErrorNotAnUnlicensedInstallation(t *testing.T) {
	empty := writeLicenseFile(t, []byte("  \n"))
	token, err := declaredLicenseToken(licenseSettings(t, "license:\n  token: ${file:"+empty+"}\n"))()
	if err == nil || !strings.Contains(err.Error(), "license.token") {
		t.Fatalf("token %q, err %v; want an error naming license.token", token, err)
	}
	_, err = configtest.Load(t, "license:\n  token: ${env:ABSENT_LICENSE_VAR}\n", nil, nil, LicenseToken, LicenseTokenFile)
	if err == nil || !strings.Contains(err.Error(), "license.token") || !strings.Contains(err.Error(), "ABSENT_LICENSE_VAR") {
		t.Fatalf("err %v; want it to name license.token and the unset variable", err)
	}
}
