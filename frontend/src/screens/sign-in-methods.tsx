import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { isOption } from "../app/options";
import { INSTALLATION_SETTINGS_KEY } from "../app/uploadlimit";
import { Button, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { Select, type SelectOption } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { ROLES, roleOptions } from "./users-invite-form";
import "./sign-in-methods.css";

/** The narrow sign-in read, keyed apart from the installation aggregate. */
const AUTHENTICATION_POLICY_KEY = ["authentication-policy"] as const;

/**
 * Which ways contacts may sign in to this installation.
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
      // BOTH caches. The write goes through the installation PATCH, so the
      // aggregate holds a copy of these providers — but the card now reads the
      // narrow projection, and invalidating only the aggregate would leave the
      // switch showing what it showed before the save.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY }),
        queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
      ]);
    },
  });
}

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

// The server refuses a larger map; mirrored here so the refusal reaches the
// admin before a request does.
const MAX_GROUP_GRANTS = 64;

function useSetGroupRoleMap() {
  const queryClient = useQueryClient();
  return useMutation({
    // The WHOLE map travels: the PATCH replaces the stored map rather than
    // merging into it, so sending one entry would silently drop every other
    // grant. An empty object is the deliberate "clear" and is sent as itself.
    // The caller passes the map as a variable rather than closing over render
    // state, so a click cannot submit rows older than the ones on screen.
    mutationFn: async (map: Record<string, string>) => {
      const { error } = await api.PATCH("/installation/settings", {
        body: { oidc_group_role_map: map },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // Both caches, for the reason useSetEnabledProviders gives: the write
      // lands on the installation aggregate, but this card reads the narrow
      // authentication-policy projection.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY }),
        queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
      ]);
    },
  });
}

// A draft grant row. Keyed by a minted id rather than by the group name,
// because the group is the thing being typed: keying on it would remount the
// input under the admin's cursor at every keystroke.
type GrantRow = Readonly<{ id: number; group: string; role: string }>;

// The six system roles, with the stored value prepended when it is not one of
// them: the contract promises a system role key, but a map written by an older
// or newer server is still this admin's to see and to keep — a Select that
// showed its placeholder instead would read as a mapping that lost its role.
function grantRoleOptions(
  role: string,
  t: ReturnType<typeof useT>,
): SelectOption[] {
  const options = roleOptions(t);
  return isOption(role, ROLES)
    ? options
    : [{ value: role, label: role }, ...options];
}

/**
 * Why this draft cannot be saved, or null. Mirrors the server's own refusals —
 * it refuses these anyway; the mirror is what turns a failed request into a
 * sentence beside the button before one is made. Order is by what the admin
 * fixes first: a blank name, then whitespace, then a duplicate, then size.
 */
function grantRefusal(
  rows: readonly GrantRow[],
  t: ReturnType<typeof useT>,
): string | null {
  if (rows.some((row) => row.group.trim() === "")) {
    return t("groupRoles.blankGroup");
  }
  if (rows.some((row) => row.group !== row.group.trim())) {
    return t("groupRoles.whitespaceGroup");
  }
  if (new Set(rows.map((row) => row.group)).size !== rows.length) {
    return t("groupRoles.duplicateGroup");
  }
  if (rows.length > MAX_GROUP_GRANTS) {
    return t("groupRoles.tooMany");
  }
  return null;
}

/**
 * The group→role grant editor: which IdP groups grant which role at corporate
 * sign-in.
 *
 * A DRAFT, saved whole. The wire setting is one map that replaces rather than
 * merges, so the rows are edited locally and one Save sends them all — the same
 * reason useSetEnabledProviders sends the whole provider list. The draft is
 * seeded from the stored map once and then owned by the admin: a background
 * refetch must not overwrite rows mid-edit, and after a save the draft already
 * IS what the server stored.
 *
 * The warning is a standing Callout rather than a tooltip because it is the
 * setting's honest cost, not a hint: the directory only ever grants here.
 */
function GroupRoleGrants({
  initial,
  canManage,
}: Readonly<{ initial: Record<string, string>; canManage: boolean }>) {
  const t = useT();
  const save = useSetGroupRoleMap();
  const [rows, setRows] = useState<readonly GrantRow[]>(() =>
    Object.entries(initial).map(([group, role], index) => ({
      id: index,
      group,
      role,
    })),
  );
  const nextId = useRef(Object.keys(initial).length);
  const refusal = grantRefusal(rows, t);
  const editRow = (id: number, edit: Partial<GrantRow>) =>
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, ...edit } : row)),
    );
  return (
    <div className="group-grants">
      <p className="t-label">{t("groupRoles.title")}</p>
      <p className="t-caption">{t("groupRoles.sub")}</p>
      <Callout
        tone="warning"
        kind="standing"
        title={t("groupRoles.grantOnlyTitle")}
      >
        <p>{t("groupRoles.grantOnly")}</p>
        <p>{t("groupRoles.adminGrant")}</p>
      </Callout>
      {save.error && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("signInMethods.saveFailed")}
        >
          {problemMessageOf(save.error, t)}
        </Callout>
      )}
      {rows.length === 0 ? (
        <p className="t-caption">{t("groupRoles.empty")}</p>
      ) : (
        <div className="group-grant-rows">
          {rows.map((row) => (
            <div className="group-grant-row" key={row.id}>
              <TextInput
                aria-label={t("groupRoles.group")}
                placeholder={t("groupRoles.groupPlaceholder")}
                value={row.group}
                autoComplete="off"
                disabled={!canManage || save.isPending}
                onChange={(event) =>
                  editRow(row.id, { group: event.target.value })
                }
              />
              <Select
                aria-label={t("groupRoles.role")}
                options={grantRoleOptions(row.role, t)}
                value={row.role}
                disabled={!canManage || save.isPending}
                onChange={(role) => editRow(row.id, { role })}
              />
              <Button
                iconOnly
                aria-label={
                  row.group.trim() === ""
                    ? t("groupRoles.remove")
                    : t("groupRoles.removeNamed", { group: row.group })
                }
                disabled={!canManage || save.isPending}
                onClick={() =>
                  setRows((current) =>
                    current.filter((each) => each.id !== row.id),
                  )
                }
              >
                <X aria-hidden="true" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <div className="form-actions">
        <Button
          disabled={!canManage || save.isPending}
          reason={
            rows.length >= MAX_GROUP_GRANTS
              ? t("groupRoles.tooMany")
              : undefined
          }
          onClick={() =>
            setRows((current) => [
              ...current,
              // Least privilege as the starting answer: a row born as Admin
              // would make the costliest grant the one a hurried save ships.
              { id: nextId.current++, group: "", role: "read_only" },
            ])
          }
        >
          {t("groupRoles.add")}
        </Button>
        <Button
          variant="primary"
          pending={save.isPending}
          disabled={!canManage}
          reason={refusal ?? undefined}
          onClick={() =>
            save.mutate(
              Object.fromEntries(rows.map((row) => [row.group, row.role])),
            )
          }
        >
          {t("groupRoles.save")}
        </Button>
      </div>
    </div>
  );
}

export function SignInMethodsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  // The group→role map GRANTS roles, so writing it is admin-only
  // (`authentication_policy:update`), not the `installation_settings:update`
  // that toggles the provider switches — an ops holder administers sign-in
  // settings but must not be able to grant itself a role through the directory.
  // Gated separately so the switches stay usable for them while the map does not.
  const canManageGrants = useCanWrite("authentication_policy", "update");
  const settings = useAuthenticationPolicy();
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
                  <Callout
                    tone="danger"
                    kind="outcome"
                    title={t("signInMethods.saveFailed")}
                  >
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
                  <p>{t("signInMethods.noneConfigured")}</p>
                )}
                <GroupRoleGrants
                  // Defaulted for the same reason `sign_in_providers` is: the
                  // field is contract-required, and a body that lost it hands
                  // over `undefined` anyway.
                  initial={current.oidc_group_role_map ?? {}}
                  canManage={canManageGrants}
                />
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
