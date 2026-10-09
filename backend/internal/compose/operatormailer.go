// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailer"
)

// OperatorMailer builds the relay transport from the deployment file, with the
// credential resolved through the key vault. The api and the worker both send
// through it, so both read the credential the same way.
//
// A password declared removed sends without AUTH, whatever username the file
// still names: the mailer authenticates whenever it has a username.
func OperatorMailer(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, cfg deployconfig.Config, lookup config.Lookup, log *slog.Logger) (mailer.SMTP, error) {
	password, err := sealedSMTPPassword(ctx, pool, vault, cfg, lookup, log)
	if err != nil {
		return mailer.SMTP{}, err
	}
	username := cfg.Email.SMTP.Username
	if cfg.Email.SMTPPasswordRemoved() {
		username = ""
	}
	return mailer.SMTP{
		Host:        cfg.Email.SMTP.Host,
		Port:        cfg.Email.SMTP.Port,
		Username:    username,
		Password:    password,
		FromAddress: cfg.Email.FromAddress,
	}, nil
}
