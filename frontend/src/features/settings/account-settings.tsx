// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { THEME_LABEL_KEYS } from "../../app/account";
import { Avatar, Button, SegmentedControl } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { IdentityFact, IdentityLine } from "../../design-system/identityline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { RoleBadge } from "../../design-system/rbac";
import { useSaveDraft } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { type Toast, useToast } from "../../design-system/toast";
import {
  LOCALES,
  type Locale,
  localeNameKey,
  useLocale,
  useT,
} from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useInstallState } from "../../lib/pwa";
import { useLogout, useMe } from "../../lib/session";
import {
  setThemeChoice,
  THEME_CHOICES,
  type ThemeChoice,
  useThemeChoice,
} from "../../lib/theme";
import { MfaSettingRow } from "../sign-in/mfacard";
import { PasswordSettingRow } from "../sign-in/passwordcard";
import { reloadReaderLanguageReads } from "./languagereads";
import { NameSettingRows, useAccountNames } from "./settings-names";
import { SignatureSettingRow } from "./settingssignaturerow";
import { InstallSettingRow } from "./thisdevice";

// The Account page as task cards, each one job a reader comes here to do. They
// cover how colleagues see you, how you sign in, the language that follows the
// account, and what this browser alone remembers. Every field and choice
// commits through the page's save bar.
export function AccountSettings() {
  // The page owns where an announcement lands, not the row that produced it.
  // A toast region between two rows would take the hairline the list draws.
  const toast = useToast();
  return (
    <>
      <ProfilePanel toast={toast} />
      <SignInPanel />
      <LanguagePanel />
      <BrowserPanel />
    </>
  );
}

function ProfilePanel({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const query = useMe();
  const names = useAccountNames(toast);
  return (
    <Panel title={t("account.profileCard")}>
      <PanelBody>
        <QueryGate query={query} pendingLabel={t("account.profileCard")}>
          {(me) => (
            <div className="settings-identity">
              {/* Both halves are required on the wire, so `|| ""` is the promise
                  that a server answering with neither costs an unnamed chip. */}
              <Avatar
                size="lg"
                identity={me.user.id}
                name={me.user.display_name || me.user.email || ""}
              />
              <div className="settings-identity-id">
                <div className="settings-identity-name">
                  <Heading size="medium">
                    {me.user.display_name || me.user.email}
                  </Heading>
                  {me.roles.map((role) => (
                    <RoleBadge key={role} roleKey={role} />
                  ))}
                </div>
                <IdentityLine>
                  <IdentityFact>{me.user.email}</IdentityFact>
                  <IdentityFact quiet>{me.workspace_name}</IdentityFact>
                </IdentityLine>
              </div>
            </div>
          )}
        </QueryGate>
      </PanelBody>
      <SettingList bleed="settings">
        <NameSettingRows names={names} />
        <SignatureSettingRow toast={toast} />
      </SettingList>
    </Panel>
  );
}

// The credential and the session, together. The password row ends every other
// session, and Sign out ends this one.
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
        <MfaSettingRow />
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

function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Locale | null>(null);
  // Written to the seat so it follows this colleague to their next browser. The
  // page switches language only once the seat kept it, so a refusal changes
  // nothing on screen.
  const remember = useMutation({
    mutationFn: async (next: Locale) => {
      unwrap(await api.PUT("/me/locale", { body: { locale: next } }), t);
      return next;
    },
    onSuccess: (next) => {
      setLocale(next);
      setDraft(null);
      void queryClient.invalidateQueries({ queryKey: ["me"] });
      void reloadReaderLanguageReads(queryClient);
    },
  });
  const refusalId = useId();
  const shown = draft ?? locale;
  useSaveDraft({
    dirty: shown !== locale,
    save: () => remember.mutateAsync(shown),
    discard: () => {
      remember.reset();
      setDraft(null);
    },
  });
  return (
    <SettingRow
      label={t("locale.switchLabel")}
      description={t("account.languageHelp")}
      control={(control) => (
        <div className="form-stack settingrow-measure">
          <Select
            {...control}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              remember.isError && refusalId,
            )}
            className="settingrow-measure"
            value={shown}
            // Narrowed through the list the options were built from, so the
            // answer is a Locale with no assertion.
            onChange={(next) => {
              const picked = LOCALES.find((option) => option === next);
              if (picked) setDraft(picked);
            }}
            // Each language name is in its own language, so each carries `lang`
            // (WCAG 2.2 AA 3.1.2); a locale code is already a BCP 47 subtag.
            options={LOCALES.map((option) => ({
              value: option,
              label: t(localeNameKey(option)),
              lang: option,
            }))}
          />
          {/* The bar carries the summary; this names the field it is about. */}
          <ErrorLine id={refusalId}>
            {remember.isError ? problemMessageOf(remember.error, t) : null}
          </ErrorLine>
        </div>
      )}
    />
  );
}

/**
 * The appearance choice, here as well as in the account menu.
 *
 * Both read `useThemeChoice` and write `setThemeChoice`. This row holds its pick
 * as a draft and paints it only on Save, so the menu never shows an unsaved theme.
 */
function AppearanceSettingRow() {
  const t = useT();
  const choice = useThemeChoice();
  const [draft, setDraft] = useState<ThemeChoice | null>(null);
  const shown = draft ?? choice;
  useSaveDraft({
    dirty: shown !== choice,
    save: async () => {
      setThemeChoice(shown);
      setDraft(null);
    },
    discard: () => setDraft(null),
  });
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
          value={shown}
          labels={labels}
          onChange={setDraft}
        />
      }
    />
  );
}
