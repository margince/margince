import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, EmptyState } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../design-system/confirmphrase";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { providerBrandName } from "../design-system/provider-mark";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { StatusBadge } from "../design-system/statusbadge";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../lib/problem";
import { INSTALLATION_SETTINGS_KEY } from "../lib/uploadlimit";
import { useCompanyPhrase } from "./company-phrase";
import {
  AUTHENTICATION_POLICY_KEY,
  isVendor,
  useOAuthApp,
  type Vendor,
} from "./oauth-app";
import type { Refusal } from "./sign-in-group-roles";
import { GroupRoleGrants } from "./sign-in-group-roles";
import "./sign-in-methods.css";

type SignInProvider = components["schemas"]["SignInProvider"];

/**
 * Which ways contacts may sign in to this installation.
 *
 * The list is what the DEPLOYMENT makes possible: an admin turns a provider off
 * or back on, but cannot add one, because a client id and secret cannot be
 * invented from a settings screen. Password is always there and is shown as
 * such, so an admin never wonders whether it was configured at all.
 */
function useSetEnabledProviders() {
  const queryClient = useQueryClient();
  return useMutation({
    // The WHOLE list travels, never a single key: the setting replaces rather
    // than merges, so sending one provider would silently turn every other one
    // off. It arrives as a variable, so a click cannot submit a stale list.
    mutationFn: async (keys: string[]) => {
      unwrap(
        await api.PATCH("/installation/settings", {
          body: { enabled_oidc_providers: keys },
        }),
      );
    },
    onSuccess: async () => {
      // The write lands on the installation aggregate, but this card reads the
      // narrow projection, so both go stale.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY }),
        queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
      ]);
    },
  });
}

/**
 * The sign-in policy, read behind `authentication_policy` rather than through
 * the installation aggregate every seeded role reads, which is what keeps the
 * page a Management+ surface rather than everybody's.
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

export function SignInMethodsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  // The group→role map GRANTS roles, so writing it is admin-only, apart from
  // the switches an ops holder may flip: it must not grant itself a role.
  const canManageGrants = useCanWrite("authentication_policy", "update");
  const settings = useAuthenticationPolicy();
  const save = useSetEnabledProviders();
  // The provider whose switch-off waits on the typed confirmation.
  const [turningOff, setTurningOff] = useState<SignInProvider | null>(null);
  const [grantsRefused, setGrantsRefused] = useState<Refusal>();
  // The switch-off's refusal is said inside its dialog while that is open.
  const refused =
    save.error && turningOff === null
      ? { title: t("signInMethods.saveFailed"), error: save.error }
      : grantsRefused;

  return (
    <Panel title={t("signInMethods.title")}>
      <PanelBody>
        <PanelIntro>{t("signInMethods.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={settings} pendingLabel={t("signInMethods.title")}>
        {(current) => {
          // Defaulted, not asserted: a body that lost a required field would
          // otherwise take the whole settings page down.
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
                        reason={
                          canManage
                            ? undefined
                            : t("signInMethods.noWriteReason")
                        }
                        // Held while ANY save is out: the list travels whole,
                        // so a second flip from the stale cache would undo
                        // the first.
                        disabled={save.isPending}
                        onChange={(next) => {
                          if (next) {
                            save.mutate([...enabledKeys, provider.key]);
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
              <PanelBody>
                {providers.length === 0 && (
                  <EmptyState>{t("signInMethods.noneConfigured")}</EmptyState>
                )}
                <GroupRoleGrants
                  // Defaulted for the reason `sign_in_providers` is.
                  initial={current.oidc_group_role_map ?? {}}
                  canManage={canManageGrants}
                  onRefusal={setGrantsRefused}
                />
              </PanelBody>
              <PanelNotices refused={refused} />
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
                      enabledKeys.filter((key) => key !== turningOff.key),
                      { onSuccess: () => setTurningOff(null) },
                    )
                  }
                />
              )}
            </>
          );
        }}
      </QueryGate>
    </Panel>
  );
}

// Whether the button is on the sign-in screen right now. The verdict is the
// server's; only the reason a chosen provider is missing is worked out here.
function ScreenLine({
  provider,
  onScreen,
}: Readonly<{ provider: SignInProvider; onScreen: boolean }>) {
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
  return isVendor(provider.key) ? (
    <VendorMissingLine vendor={provider.key} />
  ) : (
    <MissingLine
      reason={t("signInMethods.offScreenReason.unusable", {
        vendor: provider.label,
      })}
    />
  );
}

// The reason comes from the app the page already reads; a reason the server
// adds later falls through to "cannot be used right now" rather than a guess.
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
