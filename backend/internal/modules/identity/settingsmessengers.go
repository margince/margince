// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/config"
)

// CallWhatsApp is one switch for the whole installation; no seat turns the WhatsApp link on for itself.
var CallWhatsApp = config.DefineStored(config.Spec[bool]{
	Key:     "installation.call_whatsapp",
	Env:     "MARGINCE_INSTALLATION_CALL_WHATSAPP",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Show a WhatsApp chat link next to a contact's phone number that starts with a country code.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// CallTelegram is one switch for the whole installation; no seat turns the Telegram link on for itself.
var CallTelegram = config.DefineStored(config.Spec[bool]{
	Key:     "installation.call_telegram",
	Env:     "MARGINCE_INSTALLATION_CALL_TELEGRAM",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Show a Telegram chat link next to a contact's phone number that starts with a country code.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// CallZalo is one switch for the whole installation; no seat turns the Zalo link on for itself.
var CallZalo = config.DefineStored(config.Spec[bool]{
	Key:     "installation.call_zalo",
	Env:     "MARGINCE_INSTALLATION_CALL_ZALO",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Show a Zalo chat link next to a contact's phone number that starts with a country code.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

func (s *InstallationSettingsStore) readCallMessengers(ctx context.Context, out *InstallationSettings) error {
	var err error
	if out.CallWhatsApp, err = config.StoredVal(ctx, s.config, CallWhatsApp); err != nil {
		return err
	}
	if out.CallTelegram, err = config.StoredVal(ctx, s.config, CallTelegram); err != nil {
		return err
	}
	out.CallZalo, err = config.StoredVal(ctx, s.config, CallZalo)
	return err
}

func (p InstallationPatch) callMessengerWrites() ([]pendingWrite, error) {
	whatsapp, err := encodeStoredField(CallWhatsApp, p.CallWhatsApp)
	if err != nil {
		return nil, err
	}
	telegram, err := encodeStoredField(CallTelegram, p.CallTelegram)
	if err != nil {
		return nil, err
	}
	zalo, err := encodeStoredField(CallZalo, p.CallZalo)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{whatsapp, telegram, zalo}, nil
}
