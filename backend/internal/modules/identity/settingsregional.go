// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

// DateFormat changes date notation without changing stored dates or timezone boundaries.
var DateFormat = config.DefineStored(config.Spec[string]{
	Key:     "installation.date_format",
	Env:     "MARGINCE_INSTALLATION_DATE_FORMAT",
	Type:    config.String,
	Default: config.Some("locale"),
	Validate: func(value string) error {
		if !crmcontracts.InstallationSettingsDateFormat(value).Valid() {
			return errors.New("a date format is one of locale, dmy, mdy, ymd")
		}
		return nil
	},
	Doc: "How dates are written: locale, dmy, mdy or ymd.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// TimeFormat selects the displayed clock convention without changing stored instants.
var TimeFormat = config.DefineStored(config.Spec[string]{
	Key:     "installation.time_format",
	Env:     "MARGINCE_INSTALLATION_TIME_FORMAT",
	Type:    config.String,
	Default: config.Some("locale"),
	Validate: func(value string) error {
		if !crmcontracts.InstallationSettingsTimeFormat(value).Valid() {
			return errors.New("a time format is one of locale, 24h, 12h")
		}
		return nil
	},
	Doc: "How times of day are written: locale, 24h or 12h.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

func (s *InstallationSettingsStore) regionalFormats(ctx context.Context) (string, string, error) {
	dateFormat, err := config.StoredVal(ctx, s.config, DateFormat)
	if err != nil {
		return "", "", err
	}
	timeFormat, err := config.StoredVal(ctx, s.config, TimeFormat)
	return dateFormat, timeFormat, err
}
