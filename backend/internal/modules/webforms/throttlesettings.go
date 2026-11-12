// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
)

// How hard the public intake brakes a flood. The operator chooses the numbers;
// the order of the four ceilings and what each one keys on stay in Throttle.

var apiRole = []config.Role{config.RoleAPI}

// The defaults, for the settings and for a composition no file configured.
const (
	defaultPerAddress, defaultPerIP, defaultPerForm                    = 10, 5, 30
	defaultPerWorkspace, defaultPerInstallation, defaultMinSenders     = 120, 600, 10
	defaultBlockAfter, defaultCooldownMinutes, defaultMaxCooldownHours = 5, 15, 24
	defaultUnknownKeys, defaultPreflights                              = 60, 20
	defaultFormRefreshSeconds, defaultFormExpirySeconds                = 30, 120
)

var (
	ThrottlePerAddress = config.Define(config.Spec[int]{
		Key: "webforms.throttle.per_address_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PER_ADDRESS_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPerAddress), Min: config.Some(1), Max: config.Some(10_000), Roles: apiRole,
		Doc: "Posts one address may send to all web forms together in a minute before it gets 429.",
	})
	ThrottleMinSenders = config.Define(config.Spec[int]{
		Key: "webforms.throttle.min_senders", Env: "MARGINCE_WEBFORMS_THROTTLE_MIN_SENDERS", Type: config.Int,
		Default: config.Some(defaultMinSenders), Min: config.Some(1), Max: config.Some(100_000), Roles: apiRole,
		Doc: "Distinct addresses a minute over the workspace or installation limit must hold before all forms are blocked. Fewer only get 429.",
	})
	ThrottleUnknownKeys = config.Define(config.Spec[int]{
		Key: "webforms.throttle.unknown_keys_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_UNKNOWN_KEYS_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultUnknownKeys), Min: config.Some(1), Max: config.Some(100_000), Roles: apiRole,
		Doc: "Lookups of form addresses not yet known, from all senders together, in a minute. Past it such an address is answered as one that names no form.",
	})
	ThrottlePreflights = config.Define(config.Spec[int]{
		Key: "webforms.throttle.preflights_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PREFLIGHTS_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPreflights), Min: config.Some(1), Max: config.Some(10_000), Roles: apiRole,
		Doc: "OPTIONS requests a browser asks before a post, that one address may send to all web forms in a minute before it gets 429.",
	})
	ThrottleFormRefreshSeconds = config.Define(config.Spec[int]{
		Key: "webforms.throttle.form_refresh_seconds", Env: "MARGINCE_WEBFORMS_THROTTLE_FORM_REFRESH_SECONDS", Type: config.Int,
		Default: config.Some(defaultFormRefreshSeconds), Min: config.Some(1), Max: config.Some(3600), Roles: apiRole,
		Doc: "Seconds between loads of every form address the intake knows. A form made on another server is taken within it.",
	})
	ThrottleFormExpirySeconds = config.Define(config.Spec[int]{
		Key: "webforms.throttle.form_expiry_seconds", Env: "MARGINCE_WEBFORMS_THROTTLE_FORM_EXPIRY_SECONDS", Type: config.Int,
		Default: config.Some(defaultFormExpirySeconds), Min: config.Some(1), Max: config.Some(86_400), Roles: apiRole,
		Doc: "Seconds a known form address is used without reading it again, even while loads fail.",
	})
	ThrottlePerIP = config.Define(config.Spec[int]{
		Key: "webforms.throttle.per_ip_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PER_IP_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPerIP), Min: config.Some(1), Max: config.Some(10_000), Roles: apiRole,
		Doc: "Posts one address may send to one web form in a minute before it gets 429.",
	})
	ThrottlePerForm = config.Define(config.Spec[int]{
		Key: "webforms.throttle.per_form_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PER_FORM_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPerForm), Min: config.Some(1), Max: config.Some(100_000), Roles: apiRole,
		Doc: "Posts one web form takes in a minute, from every address together, before it gets 429.",
	})
	ThrottlePerWorkspace = config.Define(config.Spec[int]{
		Key: "webforms.throttle.per_workspace_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PER_WORKSPACE_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPerWorkspace), Min: config.Some(1), Max: config.Some(1_000_000), Roles: apiRole,
		Doc: "Posts all web forms of one workspace take in a minute before they get 429.",
	})
	ThrottlePerInstallation = config.Define(config.Spec[int]{
		Key: "webforms.throttle.per_installation_per_minute", Env: "MARGINCE_WEBFORMS_THROTTLE_PER_INSTALLATION_PER_MINUTE", Type: config.Int,
		Default: config.Some(defaultPerInstallation), Min: config.Some(1), Max: config.Some(10_000_000), Roles: apiRole,
		Doc: "Posts all web forms of the installation take in a minute before they get 429.",
	})
	ThrottleBlockAfterMinutes = config.Define(config.Spec[int]{
		Key: "webforms.throttle.block_after_minutes", Env: "MARGINCE_WEBFORMS_THROTTLE_BLOCK_AFTER_MINUTES", Type: config.Int,
		Default: config.Some(defaultBlockAfter), Min: config.Some(1), Max: config.Some(60), Roles: apiRole,
		Doc: "Minutes in a row over a limit after which every post is refused for a while (a block).",
	})
	ThrottleCooldownMinutes = config.Define(config.Spec[int]{
		Key: "webforms.throttle.cooldown_minutes", Env: "MARGINCE_WEBFORMS_THROTTLE_COOLDOWN_MINUTES", Type: config.Int,
		Default: config.Some(defaultCooldownMinutes), Min: config.Some(1), Max: config.Some(24 * 60), Roles: apiRole,
		Doc: "Minutes a first block lasts. Each block that follows soon after another lasts twice as long.",
	})
	ThrottleMaxCooldownHours = config.Define(config.Spec[int]{
		Key: "webforms.throttle.max_cooldown_hours", Env: "MARGINCE_WEBFORMS_THROTTLE_MAX_COOLDOWN_HOURS", Type: config.Int,
		Default: config.Some(defaultMaxCooldownHours), Min: config.Some(1), Max: config.Some(7 * 24), Roles: apiRole,
		Doc: "Longest a block lasts, in hours. A block always ends on its own.",
	})
)

// ConfigSettings are the intake throttle settings above.
func ConfigSettings() []config.Any {
	return []config.Any{
		ThrottlePerAddress, ThrottlePerIP, ThrottlePerForm, ThrottlePerWorkspace, ThrottlePerInstallation, ThrottleMinSenders,
		ThrottleUnknownKeys, ThrottlePreflights, ThrottleFormRefreshSeconds, ThrottleFormExpirySeconds,
		ThrottleBlockAfterMinutes, ThrottleCooldownMinutes, ThrottleMaxCooldownHours,
	}
}

// ThrottleLimits are the resolved settings, every one present.
type ThrottleLimits struct {
	PerAddress, PerIP, PerForm, PerWorkspace, PerInstallation int
	MinSenders, BlockAfter, UnknownKeys, Preflights           int
	Cooldown, MaxCooldown, FormRefresh, FormExpiry            time.Duration
}

// ThrottleLimitsFrom resolves the loaded settings.
func ThrottleLimitsFrom(c *config.Config) ThrottleLimits {
	return ThrottleLimits{
		PerAddress: config.Val(c, ThrottlePerAddress), MinSenders: config.Val(c, ThrottleMinSenders),
		UnknownKeys: config.Val(c, ThrottleUnknownKeys), Preflights: config.Val(c, ThrottlePreflights),
		FormRefresh: time.Duration(config.Val(c, ThrottleFormRefreshSeconds)) * time.Second,
		FormExpiry:  time.Duration(config.Val(c, ThrottleFormExpirySeconds)) * time.Second,
		PerIP:       config.Val(c, ThrottlePerIP), PerForm: config.Val(c, ThrottlePerForm),
		PerWorkspace: config.Val(c, ThrottlePerWorkspace), PerInstallation: config.Val(c, ThrottlePerInstallation),
		BlockAfter:  config.Val(c, ThrottleBlockAfterMinutes),
		Cooldown:    time.Duration(config.Val(c, ThrottleCooldownMinutes)) * time.Minute,
		MaxCooldown: time.Duration(config.Val(c, ThrottleMaxCooldownHours)) * time.Hour,
	}
}

// DefaultThrottleLimits are the limits of a composition no file configured.
func DefaultThrottleLimits() ThrottleLimits {
	return ThrottleLimits{
		PerAddress: defaultPerAddress, MinSenders: defaultMinSenders,
		UnknownKeys: defaultUnknownKeys, Preflights: defaultPreflights,
		FormRefresh: defaultFormRefreshSeconds * time.Second, FormExpiry: defaultFormExpirySeconds * time.Second,
		PerIP: defaultPerIP, PerForm: defaultPerForm, PerWorkspace: defaultPerWorkspace, PerInstallation: defaultPerInstallation,
		BlockAfter:  defaultBlockAfter,
		Cooldown:    defaultCooldownMinutes * time.Minute,
		MaxCooldown: defaultMaxCooldownHours * time.Hour,
	}
}
