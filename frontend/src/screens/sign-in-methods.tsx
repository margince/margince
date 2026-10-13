import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { INSTALLATION_SETTINGS_KEY } from "../app/uploadlimit";
import { Button, EmptyState, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select, type SelectOption } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem, unwrap } from "./common";
import { useMyMfa } from "./mfa.queries";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  useAssignableRoles,
} from "./roles.queries";
import "./sign-in-methods.css";

/** The narrow sign-in read, keyed apart from the installation aggregate. */
const AUTHENTICATION_POLICY_KEY = ["authentication-policy"] as const;

type SignInPolicyChange = Pick<
  components["schemas"]["UpdateInstallationSettingsRequest"],
  "enabled_oidc_providers" | "oidc_group_role_map" | "require_mfa"
>;

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
function useSaveSignInPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    // The change is a variable rather than closed-over render state, so a click
    // cannot submit a value older than the control it came from.
    mutationFn: async (change: SignInPolicyChange) => {
      unwrap(await api.PATCH("/installation/settings", { body: change }));
    },
    onSuccess: async () => {
      // Both caches: the aggregate holds a copy of these settings, and the
      // card reads the narrow projection.
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

// A draft grant row. Keyed by a minted id rather than by the group name,
// because the group is the thing being typed: keying on it would remount the
// input under the admin's cursor at every keystroke.
type GrantRow = Readonly<{ id: number; group: string; role: string }>;

// The roles this admin may hand out, with the stored value prepended when it is
// not among them: a role archived since the map was written, or one a reader
// who may not assign roles never receives, is still this map's answer — a
// Select that showed its placeholder instead would read as a mapping that lost
// its role.
function grantRoleOptions(
  role: string,
  t: ReturnType<typeof useT>,
  offered: readonly AssignableRole[],
): SelectOption[] {
  const options = roleOptions(t, offered);
  return options.some((option) => option.value === role)
    ? options
    : [{ value: role, label: roleLabel(t)(role) }, ...options];
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
 * A draft, saved whole: the stored map is replaced, never merged, and an empty
 * object clears it. A background refetch must not overwrite rows mid-edit.
 * The grant-only warning stands as a Callout because it is the setting's cost.
 */
function GroupRoleGrants({
  initial,
  canManage,
}: Readonly<{ initial: Record<string, string>; canManage: boolean }>) {
  const t = useT();
  const save = useSaveSignInPolicy();
  // Asked only of a reader who may write the map: the list is the invite's own
  // assignable-roles ceiling, which refuses anybody else.
  const assignable = useAssignableRoles(canManage);
  const offered = assignable.data ?? [];
  const [rows, setRows] = useState<readonly GrantRow[]>(() =>
    Object.entries(initial).map(([group, role], index) => ({
      id: index,
      group,
      role,
    })),
  );
  const nextId = useRef(Object.keys(initial).length);
  const refusal = grantRefusal(rows, t);
  const locked = !canManage || save.isPending;
  const editRow = (id: number, edit: Partial<GrantRow>) =>
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, ...edit } : row)),
    );
  const columns: DataTableColumn<GrantRow>[] = [
    {
      key: "group",
      header: t("groupRoles.group"),
      grow: true,
      render: (row) => (
        <TextInput
          aria-label={t("groupRoles.group")}
          placeholder={t("groupRoles.groupPlaceholder")}
          value={row.group}
          autoComplete="off"
          disabled={locked}
          onChange={(event) => editRow(row.id, { group: event.target.value })}
        />
      ),
    },
    {
      key: "role",
      header: t("groupRoles.role"),
      render: (row) => (
        <Select
          className="group-grants-role"
          aria-label={t("groupRoles.role")}
          options={grantRoleOptions(row.role, t, offered)}
          value={row.role}
          disabled={locked}
          onChange={(role) => editRow(row.id, { role })}
        />
      ),
    },
    {
      key: "remove",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (row) => (
        <div className="cell-actions">
          <IconAction
            icon={<X aria-hidden />}
            label={
              row.group.trim() === ""
                ? t("groupRoles.remove")
                : t("groupRoles.removeNamed", { group: row.group })
            }
            disabled={locked}
            onClick={() =>
              setRows((current) => current.filter((each) => each.id !== row.id))
            }
          />
        </div>
      ),
    },
  ];
  return (
    <Panel
      title={t("groupRoles.title")}
      actions={
        <div className="group-grants-actions">
          <Button
            disabled={locked}
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
              save.mutate({
                oidc_group_role_map: Object.fromEntries(
                  rows.map((row) => [row.group, row.role]),
                ),
              })
            }
          >
            {t("groupRoles.save")}
          </Button>
        </div>
      }
    >
      <PanelBody>
        <PanelIntro>{t("groupRoles.sub")}</PanelIntro>
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
      </PanelBody>
      {rows.length === 0 ? (
        <PanelBody>
          <EmptyState>{t("groupRoles.empty")}</EmptyState>
        </PanelBody>
      ) : (
        <DataTable
          label={t("groupRoles.title")}
          bleed
          fold
          columns={columns}
          rows={[...rows]}
          rowKey={(row) => String(row.id)}
        />
      )}
    </Panel>
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
  const save = useSaveSignInPolicy();
  const [confirmingMfa, setConfirmingMfa] = useState(false);
  // Asked only while the confirm is open: it decides one line of that dialog.
  const ownMfa = useMyMfa(confirmingMfa);

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
                        reason={t("signInMethods.passwordReason")}
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
                      description={t("signInMethods.providerHint")}
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
        error={save.error ? problemMessageOf(save.error, t) : null}
        onConfirm={() =>
          save.mutate(
            { require_mfa: true },
            { onSuccess: () => setConfirmingMfa(false) },
          )
        }
      >
        <p>{t("signInMethods.requireMfaConfirmBody")}</p>
        {ownMfa.data?.confirmed === false && (
          <p>{t("signInMethods.requireMfaConfirmSelf")}</p>
        )}
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
