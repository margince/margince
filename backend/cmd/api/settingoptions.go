// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
)

// captureConfig is the capture posture the deployment's settings decide.
// AllowTestMailbox is an operations.* kill switch, not a capture.* tuning knob,
// so it is set here rather than folded into CaptureConfigFromDeploy.
func captureConfig(cfg apiConfig, logger *slog.Logger) compose.CaptureConfig {
	captureCfg := compose.CaptureConfigFromDeploy(cfg.settings, logger)
	compose.WarnStaleRates(cfg.settings, logger)
	captureCfg.AllowTestMailbox = config.Val(cfg.settings, programsettings.AllowTestMailbox)
	return captureCfg
}

// settingOptions are the options the deployment's settings alone decide.
func settingOptions(cfg apiConfig, logger *slog.Logger) []compose.Option {
	return []compose.Option{
		compose.WithCompanyContextRollout(config.Val(cfg.settings, compose.CompanyContextRollout)),
		compose.WithListsEnabled(config.Val(cfg.settings, programsettings.ListsEnabled)),
		compose.SecurityTxtOption(cfg.settings, time.Now(), logger),
	}
}
