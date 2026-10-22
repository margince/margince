// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { Button, EmptyState, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../../design-system/confirmphrase";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { Select, type SelectOption } from "../../design-system/select";
import type { FieldLock } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { grantsAdminRole } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  useAssignableRoles,
} from "../../lib/roles.queries";
import { useSaveSignInPolicy } from "../../lib/sign-in-policy";

// The server refuses a larger map; mirrored here so the refusal reaches the
// admin before a request does.
export const MAX_GROUP_GRANTS = 64;

// A draft grant row, keyed by a minted id. Keyed by the group being typed, the
// input would remount under the admin's cursor at every keystroke.
type GrantRow = Readonly<{ id: number; group: string; role: string }>;

// The roles this admin may hand out, plus the stored role when it is not among
// them. An archived or unassignable role is still the map's answer, and a blank
// Select would read as a mapping that lost its role.
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
 * Why this draft cannot be saved, or null. It mirrors the server's refusals, so
 * the reason shows beside the button before a request is made. The order is
 * what the admin fixes first: blank, whitespace, duplicate, then size.
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

// A role select is named for its group once the group is typed.
function roleAriaLabel(group: string, t: ReturnType<typeof useT>): string {
  return group.trim() === ""
    ? t("groupRoles.role")
    : t("groupRoles.roleFor", { group });
}

// Only a new admin grant asks: re-saving one the map already held changes
// nobody's role, and the confirmation names only the groups it adds.
function newAdminGroups(
  map: Readonly<Record<string, string>>,
  initial: Readonly<Record<string, string>>,
): string[] {
  return Object.entries(map)
    .filter(([group, role]) => grantsAdminRole(role) && initial[group] !== role)
    .map(([group]) => group);
}

// A duplicate group collapses in the saved map, so it counts as a change the
// refusal then names.
function draftChanged(
  rows: readonly GrantRow[],
  initial: Readonly<Record<string, string>>,
): boolean {
  const groups = new Set(rows.map((row) => row.group));
  return (
    groups.size !== rows.length ||
    rows.length !== Object.keys(initial).length ||
    rows.some((row) => initial[row.group] !== row.role)
  );
}

// The map waiting on the typed confirmation is held as it was when Save was
// pressed, so the dialog confirms what the admin read.
function useGrantCommit(
  save: ReturnType<typeof useSaveSignInPolicy>,
  initial: Readonly<Record<string, string>>,
  draft: Record<string, string>,
  handBack: { current: boolean },
) {
  const [confirming, setConfirming] = useState<Record<string, string> | null>(
    null,
  );
  const write = (map: Record<string, string>) =>
    save.mutate(
      { oidc_group_role_map: map },
      {
        onSuccess: () => {
          handBack.current = true;
          setConfirming(null);
        },
      },
    );
  const commit = () => {
    if (newAdminGroups(draft, initial).length > 0) {
      save.reset();
      setConfirming(draft);
    } else {
      write(draft);
    }
  };
  return { confirming, setConfirming, write, commit };
}

/**
 * A draft, saved whole: the stored map is replaced, never merged, and an empty
 * object clears it. A background refetch must not overwrite rows mid-edit.
 * The grant-only warning stands as a Callout because it is the setting's cost.
 */
export function GroupRoleGrants({
  initial,
  canManage,
  lock,
}: Readonly<{
  initial: Record<string, string>;
  canManage: boolean;
  lock: FieldLock;
}>) {
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
  const addButton = useRef<HTMLButtonElement>(null);
  const firstGroup = useRef<HTMLInputElement>(null);
  const handBack = useRef(false);
  const refusal = grantRefusal(rows, t);
  const locked = !canManage || save.isPending || lock.disabled;
  // One drawn reason, which every control the lock refuses points at.
  const lockId = useId();
  const lockReasonId = lock.disabled ? lockId : undefined;
  const draft = Object.fromEntries(rows.map((row) => [row.group, row.role]));
  const dirty = draftChanged(rows, initial);
  const { confirming, setConfirming, write, commit } = useGrantCommit(
    save,
    initial,
    draft,
    handBack,
  );
  // Save leaves once the stored map matches the draft. Add stays disabled until
  // the save settles, so focus moves to it only after that render.
  useEffect(() => {
    if (!handBack.current || save.isPending) {
      return;
    }
    handBack.current = false;
    if (!dirty && document.activeElement === document.body) {
      const add = addButton.current;
      (add && !add.disabled ? add : firstGroup.current)?.focus();
    }
  }, [dirty, save.isPending]);
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
          ref={row.id === rows[0]?.id ? firstGroup : undefined}
          aria-label={t("groupRoles.group")}
          placeholder={t("groupRoles.groupPlaceholder")}
          value={row.group}
          autoComplete="off"
          aria-describedby={lockReasonId}
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
          aria-label={roleAriaLabel(row.group, t)}
          options={grantRoleOptions(row.role, t, offered)}
          value={row.role}
          aria-describedby={lockReasonId}
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
            reasonId={lockReasonId}
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
      titleAction={
        canManage && (
          <Button
            ref={addButton}
            disabled={save.isPending}
            reasonId={lockReasonId}
            reason={
              rows.length >= MAX_GROUP_GRANTS && !lock.disabled
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
        )
      }
      actions={
        canManage &&
        dirty && (
          <div className="group-grants-actions">
            <Button
              variant="primary"
              pending={save.isPending}
              reasonId={lockReasonId}
              reason={lock.disabled ? undefined : (refusal ?? undefined)}
              onClick={commit}
            >
              {t("groupRoles.save")}
            </Button>
          </div>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("groupRoles.sub")}</PanelIntro>
        {lock.disabled && (
          <p id={lockId} className="t-caption">
            {lock.reason}
          </p>
        )}
        {/* The cost of a mapping, shown once there is one to weigh. */}
        {rows.length > 0 && (
          <Callout
            tone="warning"
            kind="standing"
            title={t("groupRoles.grantOnlyTitle")}
          >
            <p>{t("groupRoles.grantOnly")}</p>
            <p>{t("groupRoles.adminGrant")}</p>
          </Callout>
        )}
        <SaveRefusal error={save.error} hidden={confirming !== null} />
        {rows.length === 0 && <EmptyState>{t("groupRoles.empty")}</EmptyState>}
      </PanelBody>
      {rows.length > 0 && (
        <DataTable
          label={t("groupRoles.title")}
          bleed
          fold
          columns={columns}
          rows={[...rows]}
          rowKey={(row) => String(row.id)}
        />
      )}
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
          onConfirm={write}
        />
      )}
    </Panel>
  );
}

// Typed, because a group mapped to Admin hands the highest role to everyone
// the directory puts in it. That happens at their next sign-in, with nobody
// asked again.
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

function SaveRefusal({
  error,
  hidden,
}: Readonly<{ error: Error | null; hidden: boolean }>) {
  const t = useT();
  if (!error || hidden) {
    return null;
  }
  return (
    <Callout tone="danger" kind="outcome" title={t("signInMethods.saveFailed")}>
      {problemMessageOf(error, t)}
    </Callout>
  );
}
