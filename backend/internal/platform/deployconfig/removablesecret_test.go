// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

const relayWithRemovedPassword = "version: 1\nemail:\n  enabled: true\n  from_address: ops@example.test\n" +
	"  smtp:\n    host: relay.example.test\n    port: 587\n    username: margince\n    password: ${none}\n"

func TestTheRelayPasswordCanBeDeclaredRemoved(t *testing.T) {
	cfg, err := Parse([]byte(relayWithRemovedPassword))
	if err != nil {
		t.Fatalf("parsing a relay whose password is declared removed: %v", err)
	}
	if !cfg.Email.SMTPPasswordRemoved() {
		t.Fatal("password: ${none} did not read as a removed credential")
	}
	got, err := cfg.Email.SMTPPassword(config.Static(nil))
	if err != nil || got != "" {
		t.Errorf("SMTPPassword = %q, %v; a removed credential resolves to nothing", got, err)
	}
}

func TestAnAbsentRelayPasswordIsNotARemovedOne(t *testing.T) {
	doc := strings.Replace(relayWithRemovedPassword, "    password: ${none}\n", "", 1)
	cfg, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parsing a relay with no password key: %v", err)
	}
	if cfg.Email.SMTPPasswordRemoved() {
		t.Error("a deployment that left the key out read as one that removed the credential")
	}
}

// Only the relay password takes the sentinel. Every other secret field refuses
// it at decode, so an operator who writes it there learns that at boot.
func TestTheRemovalSentinelIsRefusedWhereItIsNotAccepted(t *testing.T) {
	for name, doc := range map[string]string{
		"the license token": "version: 1\nlicense:\n  token: ${none}\n",
		"the bootstrap admin password": "version: 1\nbootstrap_admin:\n  email: a@b.co\n" +
			"  display_name: A\n  password: ${none}\n",
		"the relay password beside password_file": strings.Replace(relayWithRemovedPassword,
			"    password: ${none}\n", "    password: ${none}\n    password_file: /run/secrets/smtp\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(doc))
			if err == nil {
				t.Fatal("the removal sentinel was accepted where it must be refused")
			}
			// Named as the sentinel, not as a literal password to rotate.
			if !strings.Contains(err.Error(), "email.smtp.password") {
				t.Errorf("the refusal does not say which field takes the sentinel: %v", err)
			}
		})
	}
}

// A variable or file holding the sentinel's spelling is refused rather than
// read as a password of that spelling, or as a removal.
func TestASourceHoldingTheSentinelIsRefused(t *testing.T) {
	lookup := config.Static(map[string]string{smtpPasswordVar: SecretRemoved})
	email := Email{SMTP: SMTP{Password: mustRemovable(t, "${env:"+smtpPasswordVar+"}")}}
	if _, err := email.SMTPPassword(lookup); err == nil {
		t.Error("a variable holding the removal sentinel resolved as a password")
	}
}

func TestTheRemovableSecretPatternAcceptsWhatTheLoaderAccepts(t *testing.T) {
	schema := regexp.MustCompile(RemovableSecretPattern)
	for _, raw := range []string{
		"${none}", " ${none} ", "${env:MARGINCE_SMTP_PASSWORD}", "", "none", "${none:x}", "hunter2",
	} {
		var got RemovableSecret
		loaderAccepts := yaml.Unmarshal([]byte(quoted(t, raw)), &got) == nil
		if schema.MatchString(raw) != loaderAccepts {
			t.Errorf("the schema %s %q and the loader %s it", verdict(schema.MatchString(raw)), raw, verdict(loaderAccepts))
		}
	}
}

func mustRemovable(t *testing.T, raw string) RemovableSecret {
	t.Helper()
	var s RemovableSecret
	if err := yaml.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
	return s
}
