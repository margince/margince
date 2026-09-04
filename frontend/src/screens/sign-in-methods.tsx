import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import {
  INSTALLATION_SETTINGS_KEY,
  useInstallationSettings,
} from "../app/uploadlimit";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

/**
 * Which ways people may sign in to this installation.
 *
 * The list is what the DEPLOYMENT makes possible: an admin turns a provider off
 * or back on, but cannot add one, because a client id and secret cannot be
 * invented from a settings screen. The effective answer is the intersection, so
 * this screen can only ever narrow what is configured.
 *
 * The read is the SHARED installation-settings query, not a second one under the
 * same cache key: two query functions on one key let observer order decide whose
 * error and retry behaviour every reader of that key gets.
 *
 * Password is rendered and is permanently on. It is not a member of the stored
 * set at all — there is no value of the setting that removes it — so the row
 * carries a `reason` rather than a flippable switch, which is the honest way to
 * show a control that exists and cannot be moved. Hiding the row instead would
 * leave an admin wondering whether password sign-in was configured at all.
 */
function useSetEnabledProviders() {
  const queryClient = useQueryClient();
  return useMutation({
    // The WHOLE list travels, never a single key: the setting replaces rather
    // than merges, so sending one provider would silently turn every other one
    // off. The caller passes it as a variable rather than closing over render
    // state, so a click cannot submit a list older than the row it came from.
    mutationFn: async (keys: string[]) => {
      const { error } = await api.PATCH("/installation/settings", {
        body: { enabled_oidc_providers: keys },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: INSTALLATION_SETTINGS_KEY,
      });
    },
  });
}

export function SignInMethodsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  const settings = useInstallationSettings();
  const save = useSetEnabledProviders();

  return (
    <Panel title={t("signInMethods.title")}>
      <PanelBody>
        <p className="t-body">{t("signInMethods.sub")}</p>
        <QueryGate query={settings} pendingLabel={t("signInMethods.title")}>
          {(current) => {
            // Defaulted, not asserted. The field is contract-required, but a
            // body that lost one hands over `undefined` anyway, and this card
            // sits on the settings screen — dereferencing it would take the
            // whole page down over a list nobody could act on. The same reading
            // Switch's own `checked` prop documents.
            const providers = current.sign_in_providers ?? [];
            const enabledKeys = providers
              .filter((provider) => provider.enabled)
              .map((provider) => provider.key);
            return (
              <>
                {save.error && (
                  <Callout tone="danger">
                    {problemMessageOf(save.error, t)}
                  </Callout>
                )}
                <SettingList>
                  <SettingRow
                    label={t("signInMethods.password")}
                    description={t("signInMethods.passwordAlways")}
                    control={(control) => (
                      <Switch
                        label={t("signInMethods.password")}
                        labelHidden
                        checked
                        describedBy={control["aria-describedby"]}
                        reason={t("signInMethods.passwordReason")}
                        onChange={() => undefined}
                      />
                    )}
                  />
                  {providers.map((provider) => (
                    <SettingRow
                      key={provider.key}
                      label={provider.label}
                      description={t("signInMethods.providerHint")}
                      control={(control) => (
                        <Switch
                          label={provider.label}
                          labelHidden
                          checked={provider.enabled}
                          describedBy={control["aria-describedby"]}
                          pending={save.isPending}
                          // Disabled while ANY save is in flight, not just for
                          // a reader who may not write. The list travels whole,
                          // so a second flip computed from the still-stale cache
                          // would send a list that undoes the first one.
                          disabled={!canManage || save.isPending}
                          onChange={(next) =>
                            save.mutate(
                              next
                                ? [...enabledKeys, provider.key]
                                : enabledKeys.filter(
                                    (key) => key !== provider.key,
                                  ),
                            )
                          }
                        />
                      )}
                    />
                  ))}
                </SettingList>
                {providers.length === 0 && (
                  <p className="t-caption">
                    {t("signInMethods.noneConfigured")}
                  </p>
                )}
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
