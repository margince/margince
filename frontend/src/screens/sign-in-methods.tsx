import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
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
 * Password is rendered and is permanently on. It is not a member of the stored
 * set at all — there is no value of the setting that removes it — so the row
 * carries a `reason` rather than a flippable switch, which is the honest way to
 * show a control that exists and cannot be moved. Hiding the row instead would
 * leave an admin wondering whether password sign-in was configured at all.
 */
function useSignInProviders() {
  return useQuery({
    queryKey: ["installation-settings"],
    queryFn: async () => {
      const { data, error } = await api.GET("/installation/settings");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

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
        queryKey: ["installation-settings"],
      });
    },
  });
}

export function SignInMethodsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  const settings = useSignInProviders();
  const save = useSetEnabledProviders();

  return (
    <Panel title={t("signInMethods.title")}>
      <PanelBody>
        <p className="t-body">{t("signInMethods.sub")}</p>
        <QueryGate query={settings}>
          {(current) => {
            const providers = current.sign_in_providers;
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
                          disabled={!canManage}
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
