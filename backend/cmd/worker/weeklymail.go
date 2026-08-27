// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The worker's own operator-mail wiring.
//
// The relay was wired into cmd/api alone, because the mail this installation
// sent was answered from a request — a password reset, a Deal Room invitation.
// The weekly retrospective is the first message this product sends that nobody
// asked for in the moment, and unattended work runs HERE. So the worker needs
// the relay in its own hand; there is no request to carry it in on.
//
// It is the SAME relay and the same sealed credential the api role resolves,
// read the same way, so an operator configures outbound mail once and both
// roles use it.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailer"
)

// weeklyMailConfig resolves the weekly retrospective's outbound channel.
//
// IT NEVER FAILS THE BOOT, and that is deliberate. A worker exists to measure
// weeks, run capture and work the queue; a misconfigured relay must not cost an
// installation all of that over a message that is a nudge toward a screen the
// rep can already open. Every degradation is logged with what is missing, so an
// operator wondering where the weeklies went reads the answer at startup rather
// than inferring it from silence.
//
// A zero value mails nothing, which is what an installation that configured no
// operator mail asked for.
func weeklyMailConfig(
	ctx context.Context, cfg workerConfig, deployCfg deployconfig.Config,
	pool *pgxpool.Pool, logger *slog.Logger,
) compose.WeeklyMailConfig {
	if !deployCfg.Email.Enabled {
		return compose.WeeklyMailConfig{}
	}
	vault, _, err := keyvault.FromEnv(ctx, pool, config.FromOS)
	if err != nil {
		logger.WarnContext(ctx, "no weekly mail: the key vault the relay credential is sealed in is unavailable",
			"cause", err)
		return compose.WeeklyMailConfig{}
	}
	password, err := compose.SealedSMTPPassword(ctx, pool, vault, deployCfg, config.FromOS, logger)
	if err != nil {
		logger.WarnContext(ctx, "no weekly mail: the relay credential could not be resolved",
			"cause", err)
		return compose.WeeklyMailConfig{}
	}
	// The link is OPTIONAL where the relay is not. A worker booted without
	// --public-base-url still mails the week; it just cannot point at Home,
	// and MailBody omits the line rather than printing a broken URL.
	if cfg.publicBaseURL == "" {
		logger.WarnContext(ctx, "the weekly mail carries no link to Home: this worker has no --public-base-url")
	}
	return compose.WeeklyMailConfig{
		Mailer: mailer.SMTP{
			Host:        deployCfg.Email.SMTP.Host,
			Port:        deployCfg.Email.SMTP.Port,
			Username:    deployCfg.Email.SMTP.Username,
			Password:    password,
			FromAddress: deployCfg.Email.FromAddress,
		},
		PublicBaseURL: cfg.publicBaseURL,
	}
}

// weeklyMailBanner says at startup whether Monday's mail will go out, so an
// operator learns it from the boot line rather than from a rep asking.
func weeklyMailBanner(mail compose.WeeklyMailConfig) string {
	if mail.Mailer == nil {
		return "weekly review mail off (no operator mail configured)"
	}
	return fmt.Sprintf("weekly review mail on (one attempt per rep per week, link base %q)", mail.PublicBaseURL)
}
