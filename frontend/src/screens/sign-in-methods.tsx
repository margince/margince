// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useOAuthApp, type Vendor } from "../features/automations/oauth-app";
import { useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { problemMessageOf, throwProblem } from "../lib/problem";
import { GroupRoleGrants } from "./group-role-grants";
import { useMyMfa } from "./mfa.queries";
import {
  AUTHENTICATION_POLICY_KEY,
  useSaveSignInPolicy,
} from "./sign-in-policy";
import "./sign-in-methods.css";

/**
 * Which providers this installation offers, read on its own.
 *
 * NOT the installation aggregate this card used to read. That aggregate is
 * gated on `installation_settings`, which every seeded role holds — a rep reads
 * it for the base currency — so a page opening on it would put the sign-in
 * policy in front of the whole workspace. This endpoint answers the same values
 * behind `authentication_policy`, which is what makes the page a Management+
 * surface rather than everybody's.
 */
function useAuthenticationPolicy() {
  return useQuery({
    queryKey: AUTHENTICATION_POLICY_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/authentication-policy",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function isVendor(key: string): key is Vendor {
  return key === "google" || key === "microsoft";
}

// A switched-on provider stays off the sign-in screen until its OAuth app
// exists. Only a switched-on row asks; an unreadable app keeps the plain hint.
function ProviderHint({ provider }: Readonly<{ provider: Vendor }>) {
  const t = useT();
  const app = useOAuthApp(provider);
  return app.data?.source === "none"
    ? t("signInMethods.providerNoApp")
    : t("signInMethods.providerHint");
}

/**
 * Which ways people may sign in to this installation. An admin narrows what the
 * deployment configured and cannot add a provider. Password is always on, so its
 * row is a switch that refuses rather than a missing row.
 */
export function SignInMethodsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  // The group→role map GRANTS roles, so writing it is admin-only
  // (`authentication_policy:update`), not the `installation_settings:update`
  // that toggles the provider switches — an ops holder administers sign-in
  // settings but must not be able to grant itself a role through the directory.
  // Gated separately so the switches stay usable for them while the map does not.
  const canManageGrants = useCanWrite("authentication_policy", "update");
  // Reading a vendor app takes capture settings, which a custom role may lack.
  const canReadApps = useCan("capture_settings", "read");
  const settings = useAuthenticationPolicy();
  const save = useSaveSignInPolicy();
  const [confirmingMfa, setConfirmingMfa] = useState(false);
  // Asked only while the confirm is open: it decides one line of that dialog.
  const ownMfa = useMyMfa(confirmingMfa);
  // An answer that failed is no proof the admin holds a factor, so it warns.
  const selfLockedOut = !ownMfa.isPending && ownMfa.data?.confirmed !== true;

  return (
    <>
      <Panel title={t("signInMethods.title")}>
        <PanelBody>
          <PanelIntro>{t("signInMethods.sub")}</PanelIntro>
          {settings.isSuccess && save.error && !confirmingMfa && (
            <Callout
              tone="danger"
              kind="outcome"
              title={t("signInMethods.saveFailed")}
            >
              {problemMessageOf(save.error, t)}
            </Callout>
          )}
        </PanelBody>
        <QueryGate query={settings} pendingLabel={t("signInMethods.title")}>
          {(current) => {
            // Defaulted although required: a body missing it would otherwise
            // take the whole settings page down.
            const providers = current.sign_in_providers ?? [];
            const enabledKeys = providers
              .filter((provider) => provider.enabled)
              .map((provider) => provider.key);
            return (
              <>
                <SettingList bleed="settings">
                  <SettingRow
                    label={t("signInMethods.password")}
                    description={t("signInMethods.passwordAlways")}
                    control={(control) => (
                      <Switch
                        label={t("signInMethods.password")}
                        labelHidden
                        checked
                        describedBy={control["aria-describedby"]}
                        disabled
                        onChange={() => undefined}
                      />
                    )}
                  />
                  <SettingRow
                    label={t("signInMethods.requireMfa")}
                    description={t("signInMethods.requireMfaHint")}
                    control={(control) => (
                      <Switch
                        label={t("signInMethods.requireMfa")}
                        labelHidden
                        checked={current.require_mfa === true}
                        describedBy={control["aria-describedby"]}
                        pending={save.isPending}
                        disabled={!canManage || save.isPending}
                        onChange={(next) => {
                          if (!next) {
                            save.mutate({ require_mfa: false });
                            return;
                          }
                          save.reset();
                          setConfirmingMfa(true);
                        }}
                      />
                    )}
                  />
                  {providers.map((provider) => (
                    <SettingRow
                      key={provider.key}
                      label={provider.label}
                      description={
                        isVendor(provider.key) &&
                        provider.enabled &&
                        canReadApps ? (
                          <ProviderHint provider={provider.key} />
                        ) : (
                          t("signInMethods.providerHint")
                        )
                      }
                      control={(control) => (
                        <Switch
                          label={provider.label}
                          labelHidden
                          checked={provider.enabled}
                          describedBy={control["aria-describedby"]}
                          pending={save.isPending}
                          // The list is sent whole, so a second flip during a
                          // save would send the stale list and undo the first.
                          disabled={!canManage || save.isPending}
                          // The setting replaces the list, so one key alone
                          // would turn the rest off.
                          onChange={(next) =>
                            save.mutate({
                              enabled_oidc_providers: next
                                ? [...enabledKeys, provider.key]
                                : enabledKeys.filter(
                                    (key) => key !== provider.key,
                                  ),
                            })
                          }
                        />
                      )}
                    />
                  ))}
                </SettingList>
                {providers.length === 0 && (
                  <PanelBody>
                    <EmptyState>{t("signInMethods.noneConfigured")}</EmptyState>
                  </PanelBody>
                )}
              </>
            );
          }}
        </QueryGate>
      </Panel>
      <ConfirmModal
        open={confirmingMfa}
        onClose={() => {
          setConfirmingMfa(false);
          save.reset();
        }}
        title={t("signInMethods.requireMfaConfirmTitle")}
        confirmLabel={t("signInMethods.requireMfa")}
        pending={save.isPending}
        confirmDisabled={ownMfa.isPending}
        error={save.error ? problemMessageOf(save.error, t) : null}
        onConfirm={() =>
          save.mutate(
            { require_mfa: true },
            { onSuccess: () => setConfirmingMfa(false) },
          )
        }
      >
        <p>{t("signInMethods.requireMfaConfirmBody")}</p>
        {selfLockedOut && <p>{t("signInMethods.requireMfaConfirmSelf")}</p>}
      </ConfirmModal>
      {settings.data && (
        <GroupRoleGrants
          // Defaulted for the same reason `sign_in_providers` is: the field is
          // contract-required, and a body that lost it hands over `undefined`.
          initial={settings.data.oidc_group_role_map ?? {}}
          canManage={canManageGrants}
        />
      )}
    </>
  );
}
