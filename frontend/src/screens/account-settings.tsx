// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { THEME_LABEL_KEYS } from "../app/account";
import { Avatar, Button, SegmentedControl } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { RoleBadge } from "../design-system/rbac";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { type Toast, useToast } from "../design-system/toast";
import { LOCALES, type Locale, localeNameKey, useLocale, useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { useInstallState } from "../lib/pwa";
import { useLogout, useMe } from "../lib/session";
import { setThemeChoice, THEME_CHOICES, useThemeChoice } from "../lib/theme";
import { BriefDeliveryRows } from "./briefdelivery";
import { PasswordSettingRow } from "./passwordcard";
import {
  DisplayNameSettingRow,
  GreetingNameSettingRow,
} from "./settings-names";
import { SignatureSettingRow } from "./settingssignaturerow";
import { InstallSettingRow } from "./thisdevice";
import "./account-settings.css";

// The Account page as task cards, each one job a reader comes here to do: how
// colleagues see you, how you sign in, the language that follows the account,
// and what this browser alone remembers. Every row keeps the page's one row
// language, naming on the left and the control on the right.
export function AccountCard() {
  // The page owns where an announcement lands, not the row that produced it: a
  // toast region between two rows would take the hairline the list draws.
  const toast = useToast();
  return (
    <>
      <ProfilePanel toast={toast} />
      <SignInPanel />
      <LanguagePanel />
      <BrowserPanel />
      <ScheduledEmailsPanel />
    </>
  );
}

function ProfilePanel({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const query = useMe();
  return (
    <Panel title={t("account.profileCard")}>
      <PanelBody>
        <QueryGate query={query} pendingLabel={t("account.profileCard")}>
          {(me) => (
            <div className="settings-identity account-identity">
              {/* Both halves are required on the wire, so `|| ""` is the promise
                  that a server answering with neither costs an unnamed chip, not
                  the app shell this block renders inside. */}
              <Avatar
                size="lg"
                identity={me.user.id}
                name={me.user.display_name || me.user.email || ""}
              />
              <div className="settings-identity-id">
                <div className="settings-identity-name">
                  <strong className="account-identity-name">
                    {me.user.display_name || me.user.email}
                  </strong>
                  {me.roles.map((role) => (
                    <RoleBadge key={role} roleKey={role} />
                  ))}
                </div>
                {/* The address the session is bound to and the workspace it is
                    bound to it in, on one line; each its own node so either can
                    be read or matched alone. */}
                <span className="t-caption account-identity-meta">
                  <span>{me.user.email}</span>
                  <span>{me.workspace_name}</span>
                </span>
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

// The credential and the session, together: the password row is the one that
// ends every other session, and Sign out is the one that ends this one.
function SignInPanel() {
  const t = useT();
  const me = useMe();
  const logout = useLogout();
  const email = me.data?.user.email;
  return (
    <Panel title={t("account.signInCard")}>
      <SettingList bleed="settings">
        <PasswordSettingRow
          description={email ? t("account.passwordHelp", { email }) : undefined}
        />
        <SettingRow
          label={t("auth.signOut")}
          description={t("account.signOutHelp")}
          control={
            <Button
              variant="ghost"
              disabled={logout.isPending}
              onClick={() => logout.mutate()}
            >
              {t("auth.signOut")}
            </Button>
          }
        />
      </SettingList>
    </Panel>
  );
}

function LanguagePanel() {
  const t = useT();
  return (
    <Panel title={t("account.languageCard")}>
      <SettingList bleed="settings">
        <LanguageSettingRow />
      </SettingList>
    </Panel>
  );
}

// What this browser keeps and the account does not. The card is always drawn
// because Appearance always is; only the install row is absent where the
// browser cannot install.
function BrowserPanel() {
  const t = useT();
  const install = useInstallState();
  return (
    <Panel title={t("account.browserCard")}>
      <SettingList bleed="settings">
        <AppearanceSettingRow />
        <InstallSettingRow install={install} />
      </SettingList>
    </Panel>
  );
}

function ScheduledEmailsPanel() {
  const t = useT();
  return (
    <Panel title={t("account.emailsCard")}>
      <SettingList bleed="settings">
        <BriefDeliveryRows />
      </SettingList>
    </Panel>
  );
}

function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  // Written to the seat so it follows this colleague to their next browser;
  // `setLocale` keeps the local copy that renders before the request lands.
  // A failed write does not revert: the page is already in the language asked
  // for, and the next sign-in re-asks the server.
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
      description={t("account.languageHelp")}
      control={(control) => (
        <Select
          {...control}
          className="settingrow-measure"
          value={locale}
          // Narrowed through the list the options were built from, so the
          // answer is a Locale with no assertion.
          onChange={(next) => {
            const picked = LOCALES.find((option) => option === next);
            if (picked) {
              setLocale(picked);
              remember.mutate(picked);
            }
          }}
          // Each language name is in its own language, so each carries `lang`
          // (WCAG 2.2 AA 3.1.2); a locale code is already a BCP 47 subtag.
          options={LOCALES.map((option) => ({
            value: option,
            label: t(localeNameKey(option)),
            lang: option,
          }))}
        />
      )}
    />
  );
}

/**
 * The appearance choice, here as well as in the account menu.
 *
 * One state, two faces: both read `useThemeChoice` and write `setThemeChoice`,
 * so changing it in either place moves the other.
 */
function AppearanceSettingRow() {
  const t = useT();
  const choice = useThemeChoice();
  const labels = {
    light: t(THEME_LABEL_KEYS.light),
    dark: t(THEME_LABEL_KEYS.dark),
    system: t(THEME_LABEL_KEYS.system),
  };
  return (
    <SettingRow
      label={t("settings.appearance")}
      description={t("account.appearanceHelp")}
      control={
        <SegmentedControl
          label={t("settings.appearance")}
          options={THEME_CHOICES}
          value={choice}
          labels={labels}
          onChange={setThemeChoice}
        />
      }
    />
  );
}
