// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../../design-system/confirmphrase";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { providerBrandName } from "../../design-system/provider-mark";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge } from "../../design-system/statusbadge";
import { Switch } from "../../design-system/switch";
import { lockFor } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useCompanyPhrase } from "../../lib/company-phrase";
import { isVendor, useOAuthApp, type Vendor } from "../../lib/oauth-app";
import { problemMessageOf, throwProblem } from "../../lib/problem";
import {
  AUTHENTICATION_POLICY_KEY,
  useSaveSignInPolicy,
} from "../../lib/sign-in-policy";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { GroupRoleGrants } from "./group-role-grants";
import { useMyMfa } from "./mfa.queries";
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

type SignInProvider = components["schemas"]["SignInProvider"];

/**
 * Which sign-in methods this installation allows. An admin narrows what the
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
  // The policy's own read carries no locks; the installation's does, for a
  // reader who may read it. Without it, a locked write's refusal still shows.
  const canReadInstallation = useCan("installation_settings", "read");
  const locks = useInstallationSettings(canReadInstallation).data?.locks;
  const mfaLock = lockFor(locks, "identity.require_mfa", t);
  const providersLock = lockFor(locks, "identity.enabled_oidc_providers", t);
  const save = useSaveSignInPolicy();
  const [confirmingMfa, setConfirmingMfa] = useState(false);
  // The provider whose switch-off waits on the typed confirmation.
  const [turningOff, setTurningOff] = useState<SignInProvider | null>(null);
  // The confirm dialogs say their own refusal while they are open.
  const refused =
    settings.isSuccess && save.error && !confirmingMfa && turningOff === null
      ? { title: t("signInMethods.saveFailed"), error: save.error }
      : undefined;
  // Asked only while the confirm is open: it decides one line of that dialog.
  const ownMfa = useMyMfa(confirmingMfa);
  // An answer that failed is no proof the admin holds a factor, so it warns.
  const selfLockedOut = !ownMfa.isPending && ownMfa.data?.confirmed !== true;

  return (
    <>
      <Panel title={t("signInMethods.title")}>
        <PanelBody>
          <PanelIntro>{t("signInMethods.sub")}</PanelIntro>
        </PanelBody>
        <QueryGate query={settings} pendingLabel={t("signInMethods.title")}>
          {(current) => {
            // Defaulted although required: a body missing it would otherwise
            // take the whole settings page down.
            const providers = current.sign_in_providers ?? [];
            const onScreen = current.sign_in_screen ?? [];
            const enabledKeys = providers
              .filter((provider) => provider.enabled)
              .map((provider) => provider.key);
            return (
              <>
                <SettingList bleed="settings">
                  <SettingRow
                    label={t("signInMethods.password")}
                    description={t("signInMethods.passwordAlways")}
                    control={
                      <Badge icon={Lock}>{t("signInMethods.alwaysOn")}</Badge>
                    }
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
                        reason={mfaLock.disabled ? mfaLock.reason : undefined}
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
                        <>
                          {isVendor(provider.key) &&
                            t(`signInMethods.desc.${provider.key}`)}
                          <ScreenLine
                            provider={provider}
                            onScreen={onScreen.includes(provider.key)}
                            canReadApps={canReadApps}
                          />
                        </>
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
                          reason={
                            providersLock.disabled
                              ? providersLock.reason
                              : undefined
                          }
                          // The setting replaces the list, so one key alone
                          // would turn the rest off. Turning one off asks first.
                          onChange={(next) => {
                            if (next) {
                              save.mutate({
                                enabled_oidc_providers: [
                                  ...enabledKeys,
                                  provider.key,
                                ],
                              });
                            } else {
                              save.reset();
                              setTurningOff(provider);
                            }
                          }}
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
                {turningOff !== null && (
                  <TurnOffConfirm
                    provider={turningOff}
                    pending={save.isPending}
                    error={save.error ? problemMessageOf(save.error, t) : null}
                    onClose={() => {
                      setTurningOff(null);
                      save.reset();
                    }}
                    onConfirm={() =>
                      save.mutate(
                        {
                          enabled_oidc_providers: enabledKeys.filter(
                            (key) => key !== turningOff.key,
                          ),
                        },
                        { onSuccess: () => setTurningOff(null) },
                      )
                    }
                  />
                )}
              </>
            );
          }}
        </QueryGate>
        <PanelNotices refused={refused} />
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
          lock={lockFor(locks, "identity.oidc_group_role_map", t)}
        />
      )}
    </>
  );
}

// Whether the button is on the sign-in screen right now. The verdict is the
// server's; only the reason a chosen provider is missing is worked out here.
function ScreenLine({
  provider,
  onScreen,
  canReadApps,
}: Readonly<{
  provider: SignInProvider;
  onScreen: boolean;
  canReadApps: boolean;
}>) {
  const t = useT();
  if (onScreen) {
    return (
      <span className="signinmethods-status">
        <StatusBadge kind="done" label={t("signInMethods.onScreen")} />
      </span>
    );
  }
  if (!provider.enabled) {
    return (
      <span className="signinmethods-status">
        <StatusBadge
          kind="neutral"
          label={t("signInMethods.offScreen.turnedOff")}
        />
      </span>
    );
  }
  return isVendor(provider.key) && canReadApps ? (
    <VendorMissingLine vendor={provider.key} />
  ) : (
    <MissingLine
      reason={t("signInMethods.offScreenReason.unusable", {
        vendor: provider.label,
      })}
    />
  );
}

// The reason comes from the app the page already reads. A reason the server
// adds later falls through to "cannot be used right now", not a guess.
function VendorMissingLine({ vendor }: Readonly<{ vendor: Vendor }>) {
  const t = useT();
  const app = useOAuthApp(vendor).data;
  if (app === undefined) {
    return null;
  }
  const name = providerBrandName(vendor) ?? vendor;
  if (app.source === "none") {
    return (
      <MissingLine
        reason={t("signInMethods.offScreenReason.noApp", { vendor: name })}
      />
    );
  }
  if (vendor === "microsoft" && !app.tenant) {
    return (
      <MissingLine reason={t("signInMethods.offScreenReason.noDirectory")} />
    );
  }
  return (
    <MissingLine
      reason={t("signInMethods.offScreenReason.unusable", { vendor: name })}
    />
  );
}

function MissingLine({ reason }: Readonly<{ reason: string }>) {
  const t = useT();
  return (
    <span className="signinmethods-status">
      <StatusBadge kind="attention" label={t("signInMethods.offScreen")} />
      <span>{reason}</span>
    </span>
  );
}

// Typed, because everyone who signs in this way loses that way in at once.
// Turning a provider back on is one flip and asks nothing.
function TurnOffConfirm({
  provider,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  provider: SignInProvider;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}>) {
  const t = useT();
  const company = useCompanyPhrase();
  const [typed, setTyped] = useState("");
  const phraseInput = useRef<HTMLInputElement>(null);
  const phrase = t("confirmPhrase.risk");
  const vendor = providerBrandName(provider.key) ?? provider.label;
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("signInMethods.turnOffTitle", { provider: provider.label })}
      confirmLabel={t("signInMethods.turnOffConfirm", { vendor })}
      cancelLabel={t("signInMethods.keepOn")}
      confirmVariant="danger"
      confirmDisabled={!phraseMatches(typed, phrase)}
      pending={pending}
      error={error}
      initialFocusTo={() => phraseInput.current}
      onConfirm={onConfirm}
    >
      <p>{t("signInMethods.turnOffBody", { vendor, company })}</p>
      <ConfirmPhraseBox
        title={t("signInMethods.impactTitle")}
        impact={[
          t("signInMethods.impact.fallback", { vendor }),
          t("signInMethods.impact.inProgress", { vendor }),
          t("signInMethods.impact.groups", { vendor }),
          t("signInMethods.impact.connectors", { vendor }),
        ]}
        prompt={t("confirmPhrase.riskPrompt")}
        phrase={phrase}
        value={typed}
        onChange={setTyped}
        inputRef={phraseInput}
        disabled={pending}
      />
    </ConfirmModal>
  );
}
