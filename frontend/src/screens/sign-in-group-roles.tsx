// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { Button, Disclosure, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../design-system/confirmphrase";
import { DataTable } from "../design-system/datatable";
import { IconAction } from "../design-system/iconaction";
import { Select, type SelectOption } from "../design-system/select";
import { usePlural, useT } from "../i18n";
import { grantsAdminRole } from "../lib/capability";
import { problemMessageOf, unwrap } from "../lib/problem";
import { INSTALLATION_SETTINGS_KEY } from "../lib/uploadlimit";
import { AUTHENTICATION_POLICY_KEY } from "./oauth-app";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  useAssignableRoles,
} from "./roles.queries";

// The server refuses a larger map; mirrored here so the refusal reaches the
// admin before a request does.
const MAX_GROUP_GRANTS = 64;

function useSetGroupRoleMap() {
  const queryClient = useQueryClient();
  return useMutation({
    // The WHOLE map travels: the PATCH replaces the stored map rather than
    // merging into it, so sending one entry would silently drop every other
    // grant. An empty object is the deliberate "clear" and is sent as itself.
    mutationFn: async (map: Record<string, string>) => {
      unwrap(
        await api.PATCH("/installation/settings", {
          body: { oidc_group_role_map: map },
        }),
      );
    },
    onSuccess: async () => {
      // The write lands on the installation aggregate, but this page reads the
      // narrow authentication-policy projection, so both go stale.
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

// The roles this admin may hand out, with the stored value prepended when it is
// not among them: a role archived since the map was written is still this
// map's answer, and a placeholder would read as a mapping that lost its role.
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
 * Why this draft cannot be saved, or null. Mirrors the server's own refusals,
 * so the reason is a sentence beside the button before a request is made.
 * Ordered by what the admin fixes first.
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

// Only a NEW admin grant asks: re-saving one the map already held changes
// nobody's role, and the confirmation names only the groups it adds.
function newAdminGroups(
  map: Readonly<Record<string, string>>,
  initial: Readonly<Record<string, string>>,
): string[] {
  return Object.entries(map)
    .filter(([group, role]) => grantsAdminRole(role) && initial[group] !== role)
    .map(([group]) => group);
}

/**
 * Which directory groups hand which role to the members invited from them, behind
 * a closed disclosure because few installations map any.
 *
 * A DRAFT, saved whole: the setting is one map that replaces rather than
 * merges. The draft is seeded from the stored map once and then owned by the
 * admin, so a background refetch never overwrites rows mid-edit.
 */
export function GroupRoleGrants({
  initial,
  canManage,
}: Readonly<{ initial: Record<string, string>; canManage: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const stored = Object.keys(initial).length;
  return (
    <Disclosure
      summary={
        <>
          {t("groupRoles.disclosure")}{" "}
          <span className="t-caption">
            {stored === 0
              ? t("groupRoles.countNone")
              : plural("groupRoles.count", stored, { count: String(stored) })}
          </span>
        </>
      }
    >
      <GrantEditor initial={initial} canManage={canManage} />
    </Disclosure>
  );
}

function GrantEditor({
  initial,
  canManage,
}: Readonly<{ initial: Record<string, string>; canManage: boolean }>) {
  const t = useT();
  const save = useSetGroupRoleMap();
  // Asked only of a reader who may write the map: the list is the invite's own
  // assignable-roles ceiling, which refuses anybody else.
  const offered = useAssignableRoles(canManage).data ?? [];
  const [rows, setRows] = useState<readonly GrantRow[]>(() =>
    Object.entries(initial).map(([group, role], index) => ({
      id: index,
      group,
      role,
    })),
  );
  // The map waiting on the typed confirmation, held as it was when Save was
  // pressed so the dialog confirms exactly what the admin read.
  const [confirming, setConfirming] = useState<Record<string, string> | null>(
    null,
  );
  const nextId = useRef(Object.keys(initial).length);
  const refusal = grantRefusal(rows, t);
  const locked = !canManage || save.isPending;
  const editRow = (id: number, edit: Partial<GrantRow>) =>
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, ...edit } : row)),
    );
  function submit() {
    const map = Object.fromEntries(rows.map((row) => [row.group, row.role]));
    if (newAdminGroups(map, initial).length > 0) {
      setConfirming(map);
    } else {
      save.mutate(map);
    }
  }
  return (
    <div className="group-grants">
      <p className="t-body">{t("groupRoles.sub")}</p>
      <Callout
        tone="warning"
        kind="standing"
        title={t("groupRoles.grantOnlyTitle")}
      >
        <p>{t("groupRoles.grantOnly")}</p>
        <p>{t("groupRoles.adminGrant")}</p>
      </Callout>
      {save.error && confirming === null && (
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
        <DataTable
          label={t("groupRoles.disclosure")}
          fold
          rows={[...rows]}
          rowKey={(row) => String(row.id)}
          columns={[
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
                  onChange={(event) =>
                    editRow(row.id, { group: event.target.value })
                  }
                />
              ),
            },
            {
              key: "role",
              header: t("groupRoles.role"),
              render: (row) => (
                <Select
                  className="group-grants-role"
                  aria-label={
                    row.group.trim() === ""
                      ? t("groupRoles.role")
                      : t("groupRoles.roleFor", { group: row.group })
                  }
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
                      setRows((current) =>
                        current.filter((each) => each.id !== row.id),
                      )
                    }
                  />
                </div>
              ),
            },
          ]}
        />
      )}
      <p className="t-caption">{t("groupRoles.help")}</p>
      <div className="form-actions">
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
          reason={
            canManage ? (refusal ?? undefined) : t("groupRoles.noWriteReason")
          }
          onClick={submit}
        >
          {t("groupRoles.save")}
        </Button>
      </div>
      {/* Mounted only while asking, so the typed phrase never outlives it. */}
      {confirming !== null && (
        <AdminGrantConfirm
          groups={newAdminGroups(confirming, initial)}
          map={confirming}
          pending={save.isPending}
          error={save.error ? problemMessageOf(save.error, t) : null}
          onClose={() => {
            setConfirming(null);
            save.reset();
          }}
          onConfirm={(map) =>
            save.mutate(map, { onSuccess: () => setConfirming(null) })
          }
        />
      )}
    </div>
  );
}

// Typed, because a group mapped to Admin hands the highest role to everyone the
// directory puts in it, at their next sign-in, with nobody asked again.
function AdminGrantConfirm({
  groups,
  map,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  groups: readonly string[];
  map: Record<string, string>;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: (map: Record<string, string>) => void;
}>) {
  const t = useT();
  const [typed, setTyped] = useState("");
  const phraseInput = useRef<HTMLInputElement>(null);
  const phrase = t("confirmPhrase.risk");
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("groupRoles.adminConfirmTitle", { group: groups.join(", ") })}
      confirmLabel={t("groupRoles.adminConfirm")}
      confirmVariant="danger"
      confirmDisabled={!phraseMatches(typed, phrase)}
      pending={pending}
      error={error}
      initialFocusTo={() => phraseInput.current}
      onConfirm={() => onConfirm(map)}
    >
      <ConfirmPhraseBox
        title={t("groupRoles.adminImpactTitle")}
        impact={[
          t("groupRoles.adminImpact.signIn"),
          t("groupRoles.adminImpact.reach"),
          t("groupRoles.adminImpact.keep"),
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
