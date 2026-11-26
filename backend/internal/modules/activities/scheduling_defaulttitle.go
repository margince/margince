// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

type storedSchedulingProfile struct {
	crmcontracts.SchedulingProfile
	DefaultTitle bool `json:"_default_title,omitempty"` //nolint:tagliatelle // The private envelope key cannot collide with a public contract field.
}

func schedulingProfileToStore(ctx context.Context, profile crmcontracts.SchedulingProfile, before *storedSchedulingProfile, canonicalDefault string) storedSchedulingProfile {
	defaultTitle := profile.UseDefaultTitle != nil && *profile.UseDefaultTitle
	if profile.UseDefaultTitle == nil && before != nil && before.DefaultTitle {
		displayDefault := systemtext.Copy("booking.lets_meet", systemtext.Locale(ctx))
		defaultTitle = profile.Title == before.Title || profile.Title == displayDefault
	}
	if defaultTitle && before != nil && before.DefaultTitle {
		canonicalDefault = before.Title
	}
	profile.UseDefaultTitle = nil
	if defaultTitle {
		profile.Title = canonicalDefault
	}
	return storedSchedulingProfile{SchedulingProfile: profile, DefaultTitle: defaultTitle}
}
