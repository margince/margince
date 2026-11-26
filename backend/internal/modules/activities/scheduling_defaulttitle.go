// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

type storedSchedulingProfile struct {
	crmcontracts.SchedulingProfile
	DefaultTitle bool `json:"_default_title,omitempty"` //nolint:tagliatelle // The private envelope key cannot collide with a public contract field.
}

// UnmarshalJSON reads a page on the default title in today's wording of it. A page
// stored under an earlier wording then shows, compares and saves like one stored now.
func (p *storedSchedulingProfile) UnmarshalJSON(raw []byte) error {
	type plain storedSchedulingProfile
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if decoded.DefaultTitle {
		decoded.Title = systemtext.Reworded("booking.lets_meet", decoded.Title)
	}
	*p = storedSchedulingProfile(decoded)
	return nil
}

func schedulingProfileToStore(ctx context.Context, profile crmcontracts.SchedulingProfile, before *storedSchedulingProfile, canonicalDefault string) storedSchedulingProfile {
	defaultTitle := profile.UseDefaultTitle != nil && *profile.UseDefaultTitle
	if profile.UseDefaultTitle == nil && before != nil && before.DefaultTitle {
		defaultTitle = profile.Title == before.Title || systemtext.Says("booking.lets_meet", systemtext.Locale(ctx), profile.Title)
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
