// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// DateFormat changes date notation without changing stored dates or timezone boundaries.
var DateFormat = settings.Define[string]("installation.date_format", installationSettingsObject, "update", "locale", func(value string) error {
	if !crmcontracts.InstallationSettingsDateFormat(value).Valid() {
		return fmt.Errorf("a date format is one of locale, dmy, mdy, ymd")
	}
	return nil
}).AsInstallationIdentity()

// TimeFormat selects the displayed clock convention without changing stored instants.
var TimeFormat = settings.Define[string]("installation.time_format", installationSettingsObject, "update", "locale", func(value string) error {
	if !crmcontracts.InstallationSettingsTimeFormat(value).Valid() {
		return fmt.Errorf("a time format is one of locale, 24h, 12h")
	}
	return nil
}).AsInstallationIdentity()

func (s *InstallationSettingsStore) regionalFormats(ctx context.Context) (string, string, error) {
	dateFormat, err := settings.Get(ctx, s.settings, DateFormat)
	if err != nil {
		return "", "", err
	}
	timeFormat, err := settings.Get(ctx, s.settings, TimeFormat)
	return dateFormat, timeFormat, err
}
