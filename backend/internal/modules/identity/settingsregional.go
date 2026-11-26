// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/datenotation"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
}, config.Store{Cacheable: true, RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true, MachineryApplied: true})

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
}, config.Store{Cacheable: true, RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true, MachineryApplied: true})

// DateNotationTx reads how the installation writes dates, for server-written
// text shown beside the app's own. It takes no read gate, for the timezone's
// reason: every date the app shows discloses the notation anyway.
func DateNotationTx(ctx context.Context, tx pgx.Tx, lang textlang.Lang) (datenotation.Notation, error) {
	dateFormat, timeFormat, err := config.StoredApplyPairTx(ctx, tx, config.Bound(), DateFormat, TimeFormat)
	if err != nil {
		return datenotation.Notation{}, err
	}
	zone, err := ZoneAppliedTx(ctx, tx)
	if err != nil {
		return datenotation.Notation{}, err
	}
	return datenotation.Notation{Lang: lang, Date: dateFormat, Time: timeFormat, Zone: zone}, nil
}

func (s *InstallationSettingsStore) regionalFormats(ctx context.Context) (string, string, error) {
	dateFormat, err := config.StoredVal(ctx, s.config, DateFormat)
	if err != nil {
		return "", "", err
	}
	timeFormat, err := config.StoredVal(ctx, s.config, TimeFormat)
	return dateFormat, timeFormat, err
}
