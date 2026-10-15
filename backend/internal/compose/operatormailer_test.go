// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

var mailSettingsUnderTest = []config.Any{EmailEnabled, EmailFromAddress, SMTPHost, SMTPPort, SMTPUsername, SMTPPassword, SMTPPasswordFile}

func loadMail(t *testing.T, body string, environ ...string) (*config.Config, error) {
	t.Helper()
	return configtest.Load(t, body, environ, MailFaults, mailSettingsUnderTest...)
}

func mustLoadMail(t *testing.T, body string, environ ...string) *config.Config {
	t.Helper()
	cfg, err := loadMail(t, body, environ...)
	if err != nil {
		t.Fatalf("loading %q: %v", body, err)
	}
	return cfg
}

func smtpBlock(lines string) string { return "email:\n  smtp:\n    host: mail.example.test\n" + lines }

// The relay credential across the states a relay can be in. "No credential at
// all" is an unauthenticated relay, a real deployment, and not a missing secret.
func TestTheRelayPasswordResolvesEachConfiguredSource(t *testing.T) {
	mounted := filepath.Join(t.TempDir(), "smtp-password")
	if err := os.WriteFile(mounted, []byte("from-the-secret-store\n"), 0o600); err != nil {
		t.Fatalf("writing the mounted secret: %v", err)
	}
	for name, tc := range map[string]struct{ lines, want string }{
		"a reference to the environment": {"    password: ${env:RELAY_SECRET}\n", "from-the-environment"},
		"a reference to a mounted file":  {"    password: ${file:" + mounted + "}\n", "from-the-secret-store"},
		// The older spelling keeps working, so a deployment that names a path
		// boots unchanged.
		"the original password_file spelling": {"    password_file: " + mounted + "\n", "from-the-secret-store"},
		"no credential at all":                {"", ""},
	} {
		got, err := declaredSMTPPassword(mustLoadMail(t, smtpBlock(tc.lines), "RELAY_SECRET=from-the-environment"))
		if err != nil || got != tc.want {
			t.Errorf("%s: password %q, err %v; want %q", name, got, err, tc.want)
		}
	}
}

// The reference is the form an operator migrating forward chose to write.
// Reading the file it replaced would keep the old credential in use.
func TestTheReferenceWinsOverTheLegacyPath(t *testing.T) {
	stale := filepath.Join(t.TempDir(), "stale")
	if err := os.WriteFile(stale, []byte("the-old-credential"), 0o600); err != nil {
		t.Fatalf("writing the stale file: %v", err)
	}
	cfg := mustLoadMail(t, smtpBlock("    password: ${env:RELAY_SECRET}\n    password_file: "+stale+"\n"), "RELAY_SECRET=the-new-credential")
	if got, err := declaredSMTPPassword(cfg); err != nil || got != "the-new-credential" {
		t.Fatalf("password %q, err %v; the reference must win over password_file", got, err)
	}
}

func TestTheRelayPasswordCanBeDeclaredRemoved(t *testing.T) {
	removed := mustLoadMail(t, smtpBlock("    username: margince\n    password: ${none}\n"))
	if !smtpPasswordRemoved(removed) {
		t.Fatal("password: ${none} did not read as a removed credential")
	}
	if got, err := declaredSMTPPassword(removed); err != nil || got != "" {
		t.Errorf("password %q, err %v; a removed credential resolves to nothing", got, err)
	}
	if smtpPasswordRemoved(mustLoadMail(t, smtpBlock("    username: margince\n"))) {
		t.Error("a deployment that left the key out read as one that removed the credential")
	}
}

// The file is the most-pasted artefact an installation has, so the relay
// password is refused as a literal and never repeated back.
func TestALiteralRelayPasswordIsRefused(t *testing.T) {
	_, err := loadMail(t, smtpBlock("    password: hunter2-literal\n"))
	if err == nil || strings.Contains(err.Error(), "hunter2-literal") {
		t.Fatalf("err %v; want a refusal that does not echo the value", err)
	}
}

// An enabled relay with nowhere to send, or nobody to send as, fails the boot.
// So does a password both declared absent and named by a file.
func TestTheRelayIsCheckedAsAWhole(t *testing.T) {
	enabled := "email:\n  enabled: true\n  from_address: ops@example.test\n  smtp:\n    host: h\n    port: 587\n"
	if _, err := loadMail(t, enabled); err != nil {
		t.Fatalf("a complete relay was refused: %v", err)
	}
	for name, tc := range map[string]struct{ body, want string }{
		"email without smtp":     {"email: { enabled: true, from_address: a@b.co }\n", "email.smtp.host"},
		"smtp port out of range": {"email: { enabled: true, from_address: a@b.co, smtp: { host: h, port: 70000 } }\n", "email.smtp.port"},
		"no sender":              {"email: { enabled: true, smtp: { host: h, port: 587 } }\n", "email.from_address"},
		"absent beside a file":   {smtpBlock("    password: ${none}\n    password_file: /run/secrets/smtp\n"), "keep one of the two"},
	} {
		if _, err := loadMail(t, tc.body); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v; want it to name %q", name, err, tc.want)
		}
	}
	if _, err := loadMail(t, "email: { enabled: false, smtp: { port: 70000 } }\n"); err != nil {
		t.Errorf("a disabled relay is not checked, so a half-written block can wait: %v", err)
	}
}
