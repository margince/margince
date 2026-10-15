// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The relay for mail the installation writes by itself. Off,
// every such message is not sent and the forgot-password flow is absent
// rather than broken.
var (
	EmailEnabled = config.Define(config.Spec[bool]{
		Key: "email.enabled", Env: "MARGINCE_EMAIL_ENABLED", Type: config.Bool, Default: config.Some(false), Roles: bothRoles,
		Doc: "Send the installation's own mail through the SMTP relay below; needs --public-base-url.",
	})
	EmailFromAddress = config.Define(config.Spec[string]{
		Key: "email.from_address", Env: "MARGINCE_EMAIL_FROM_ADDRESS", Type: config.String, Roles: bothRoles,
		Doc: "Sender address of the installation's own mail; required when email.enabled.",
	})
	SMTPHost = config.Define(config.Spec[string]{
		Key: "email.smtp.host", Env: "MARGINCE_EMAIL_SMTP_HOST", Type: config.String, Roles: bothRoles,
		Doc: "SMTP relay host; required when email.enabled.",
	})
	SMTPPort = config.Define(config.Spec[int]{
		Key: "email.smtp.port", Env: "MARGINCE_EMAIL_SMTP_PORT", Type: config.Int, Default: config.Some(0), Roles: bothRoles,
		Doc: "SMTP relay port, 1 to 65535 when email.enabled.",
	})
	SMTPUsername = config.Define(config.Spec[string]{
		Key: "email.smtp.username", Env: "MARGINCE_EMAIL_SMTP_USERNAME", Type: config.String, Roles: bothRoles,
		Doc: "SMTP relay user; unset sends without authenticating.",
	})
	SMTPPassword = config.Define(config.Spec[string]{
		Key: "email.smtp.password", Env: "MARGINCE_EMAIL_SMTP_PASSWORD", Type: config.String, Secret: true, AllowNone: true, Roles: bothRoles,
		Doc: "SMTP relay password, sealed into the key vault at boot; ${none} deletes the sealed copy and sends without one.",
	})
	SMTPPasswordFile = config.Define(config.Spec[string]{
		Key: "email.smtp.password_file", Env: "MARGINCE_EMAIL_SMTP_PASSWORD_FILE", Type: config.String, Roles: bothRoles,
		Doc: "Path to a file holding the SMTP relay password; email.smtp.password wins over it.",
	})
)

// smtpPasswordLimit bounds the password file: anything larger is a path typed
// wrong.
const smtpPasswordLimit = 64 << 10

// MailFaults are the relay checks that read two settings: an enabled relay
// needs somewhere to send and someone to send as, and a password cannot be
// both declared absent and named by a file.
func MailFaults(c *config.Config) []error {
	var faults []error
	if _, origin := config.ValWithSource(c, SMTPPassword); origin.None && config.Val(c, SMTPPasswordFile) != "" {
		faults = append(faults, errors.New("email.smtp.password declares the relay password absent while "+
			"email.smtp.password_file names one — keep one of the two"))
	}
	if !config.Val(c, EmailEnabled) {
		return faults
	}
	if config.Val(c, SMTPHost) == "" {
		faults = append(faults, errors.New("email.enabled requires email.smtp.host"))
	}
	if port := config.Val(c, SMTPPort); port < 1 || port > 65535 {
		faults = append(faults, errors.New("email.enabled requires email.smtp.port between 1 and 65535"))
	}
	if _, err := values.ParseEmail(config.Val(c, EmailFromAddress)); err != nil {
		faults = append(faults, fmt.Errorf("email.from_address: %w", err))
	}
	return faults
}

// smtpPasswordRemoved is `password: ${none}`: the sealed copy is deleted and
// the relay is used without one.
func smtpPasswordRemoved(c *config.Config) bool {
	_, origin := config.ValWithSource(c, SMTPPassword)
	return origin.None
}

// declaredSMTPPassword is the credential the deployment declares: the
// reference, else the older password_file, else none, which is an
// unauthenticated relay rather than a mistake.
func declaredSMTPPassword(c *config.Config) (string, error) {
	password, origin := config.ValWithSource(c, SMTPPassword)
	if origin.Source != config.SourceUnset {
		return password, nil
	}
	path, pathOrigin := config.ValWithSource(c, SMTPPasswordFile)
	if path == "" {
		return "", nil
	}
	return readDeclaredFile(pathOrigin.Name, path, smtpPasswordLimit)
}

// OperatorMailer builds the relay transport from the deployment's settings,
// with the credential resolved through the key vault. The api and the worker
// both send through it, so both read the credential the same way.
//
// A password declared removed sends without AUTH, whatever username the file
// still names: the mailer authenticates whenever it has a username.
func OperatorMailer(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, c *config.Config, log *slog.Logger) (mailer.SMTP, error) {
	password, err := sealedSMTPPassword(ctx, pool, vault, c, log)
	if err != nil {
		return mailer.SMTP{}, err
	}
	username := config.Val(c, SMTPUsername)
	if smtpPasswordRemoved(c) {
		username = ""
	}
	return mailer.SMTP{
		Host:        config.Val(c, SMTPHost),
		Port:        config.Val(c, SMTPPort),
		Username:    username,
		Password:    password,
		FromAddress: config.Val(c, EmailFromAddress),
	}, nil
}
