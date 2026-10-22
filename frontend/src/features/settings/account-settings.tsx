// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../../api/client";
import { THEME_LABEL_KEYS } from "../../app/account";
import { Avatar, Button } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { RoleBadge } from "../../design-system/rbac";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import {
  LOCALES,
  type Locale,
  localeNameKey,
  useLocale,
  useT,
} from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useLogout, useMe } from "../../lib/session";
import { setThemeChoice, THEME_CHOICES, useThemeChoice } from "../../lib/theme";
import { MfaSettingRow } from "../sign-in/mfacard";
import { PasswordSettingRow } from "../sign-in/passwordcard";
import {
  DisplayNameSettingRow,
  GreetingNameSettingRow,
} from "./settings-names";
import { SignatureSettingRow } from "./settingssignaturerow";
import { ThisDevicePanel } from "./thisdevice";

export function AccountSettings() {
  return (
    <>
      <ProfilePanel />
      <SecurityPanel />
      <PreferencesPanel />
      <ThisDevicePanel />
    </>
  );
}

function ProfilePanel() {
  const t = useT();
  const query = useMe();
  // One toast for the panel, so an announcement never stands between two rows.
  const toast = useToast();
  return (
    <Panel title={t("settings.profileTitle")}>
      <PanelBody>
        <QueryGate query={query} pendingLabel={t("settings.profileTitle")}>
          {(me) => (
            <div className="settings-identity">
              {/* An empty name costs an unnamed chip rather than the page. */}
              <Avatar
                identity={me.user.id}
                name={me.user.display_name || me.user.email || ""}
              />
              <div className="settings-identity-id">
                <div className="settings-identity-name">
                  <strong>{me.user.display_name || me.user.email}</strong>
                  {me.roles.map((role) => (
                    <RoleBadge key={role} roleKey={role} />
                  ))}
                </div>
                <span className="t-caption">{me.user.email}</span>
                <span className="t-caption">{me.workspace_name}</span>
              </div>
            </div>
          )}
        </QueryGate>
      </PanelBody>
      <SettingList bleed="settings">
        <DisplayNameSettingRow toast={toast} />
        <GreetingNameSettingRow toast={toast} />
        <SignatureSettingRow toast={toast} />
      </SettingList>
    </Panel>
  );
}

function SecurityPanel() {
  const t = useT();
  const logout = useLogout();
  return (
    <Panel
      title={t("settings.securityTitle")}
      titleAction={
        <Button disabled={logout.isPending} onClick={() => logout.mutate()}>
          {t("auth.signOut")}
        </Button>
      }
    >
      <SettingList bleed="settings">
        <PasswordSettingRow />
        <MfaSettingRow />
      </SettingList>
    </Panel>
  );
}

function PreferencesPanel() {
  const t = useT();
  return (
    <Panel title={t("settings.preferencesTitle")}>
      <SettingList bleed="settings">
        <LanguageSettingRow />
        <AppearanceSettingRow />
      </SettingList>
    </Panel>
  );
}

// The same choice as the account menu's Theme: both read and write lib/theme.ts.
function AppearanceSettingRow() {
  const t = useT();
  const choice = useThemeChoice();
  return (
    <SettingRow
      label={t("settings.appearance")}
      description={t("settings.appearanceHelp")}
      control={(control) => (
        <Select
          {...control}
          className="settingrow-measure"
          value={choice}
          onChange={(next) => {
            const picked = THEME_CHOICES.find((option) => option === next);
            if (picked) {
              setThemeChoice(picked);
            }
          }}
          options={THEME_CHOICES.map((option) => ({
            value: option,
            label: t(THEME_LABEL_KEYS[option]),
          }))}
        />
      )}
    />
  );
}

function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  const refusalId = useId();
  // Saved to the seat so it follows the reader to their next browser. A failed
  // write keeps the language they picked; the next sign-in asks the server again.
  const remember = useMutation({
    mutationFn: async (next: Locale) => {
      unwrap(await api.PUT("/me/locale", { body: { locale: next } }), t);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  return (
    <SettingRow
      label={t("locale.switchLabel")}
      description={t("settings.languageHelp")}
      control={(control) => (
        <div className="form-stack settingrow-measure">
          <Select
            {...control}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              remember.isError && refusalId,
            )}
            value={locale}
            onChange={(next) => {
              const picked = LOCALES.find((option) => option === next);
              if (picked) {
                setLocale(picked);
                remember.mutate(picked);
              }
            }}
            // Each name is in its own language, so each option carries `lang`
            // (WCAG 2.2 AA 3.1.2).
            options={LOCALES.map((option) => ({
              value: option,
              label: t(localeNameKey(option)),
              lang: option,
            }))}
          />
          <ErrorLine id={refusalId} error={remember.error} />
        </div>
      )}
    />
  );
}
