// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Disclosure, Radio } from "../design-system/atoms";
import { NamePrompt } from "../design-system/nameprompt";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { stable } from "../format/collate";
import {
  CORE_OBJECTS,
  GRANT_AREA_NAME,
  GRANT_AREAS,
  objectLabel,
} from "../format/rbacobjects";
import { useT } from "../i18n";
import type { ExtensionUnit } from "./extensions.queries";
import { GrantMatrix, type GrantMatrixRow, grantOf } from "./grantmatrix";
import {
  type Role,
  type RowScope,
  roleLabel,
  useMoveRole,
  useSetRoleGrant,
  useUpdateRole,
} from "./roles.queries";
import { AccessPreviewPanel } from "./users-access";

// Every control writes as it is used, with the version the reader saw.

const SCOPES: readonly RowScope[] = ["own", "team", "all"];

/** One named run of the grid: an area of the product, or an extension unit. */
export type GrantGroup = Readonly<{
  key: string;
  title: string;
  objects: readonly GrantObject[];
}>;

/** An object a row is drawn for. `known` is false where the key is its only name. */
export type GrantObject = Readonly<{
  object: string;
  name: string;
  known: boolean;
}>;

/**
 * The grid's rows, by area, read off the directory and not off the client's
 * table. So an object a newer server added still shows, under "Other". So does
 * one a removed unit left behind, for an operator to find and clear.
 */
export function grantGroups(
  directory: readonly Role[],
  units: readonly ExtensionUnit[],
  t: ReturnType<typeof useT>,
): GrantGroup[] {
  const all = new Set<string>();
  for (const role of directory) {
    for (const object of Object.keys(role.objects)) {
      all.add(object);
    }
  }
  for (const unit of units) {
    for (const object of unit.rbac_objects) {
      all.add(object);
    }
  }
  const core = GRANT_AREAS.map((area) => ({
    key: area,
    title: t(GRANT_AREA_NAME[area]),
    objects: Object.entries(CORE_OBJECTS)
      .filter(([object, named]) => named.area === area && all.has(object))
      .map(([object, named]) => ({
        object,
        name: t(named.label),
        known: true,
      })),
  }));
  const claimed = new Set([
    ...core.flatMap((group) => group.objects.map((row) => row.object)),
    ...units.flatMap((unit) => unit.rbac_objects),
  ]);
  const extensions = [...units]
    .sort((a, b) => stable(a.name, b.name))
    .map((unit) => ({
      key: `ext:${unit.name}`,
      title: t("roles.group.extension", { name: unit.name }),
      objects: unit.rbac_objects.map((object) => ({
        object,
        name: objectLabel(unit.name, object),
        known: true,
      })),
    }));
  const other = {
    key: "other",
    title: t("roles.group.other"),
    objects: [...all]
      .filter((object) => !claimed.has(object))
      .sort(stable)
      .map((object) => ({ object, name: object, known: false })),
  };
  return [...core, ...extensions, other].filter(
    (group) => group.objects.length > 0,
  );
}

/** The three writes a role takes, and whether one of them is out. */
export function useRoleWrites() {
  const update = useUpdateRole();
  const setGrant = useSetRoleGrant();
  const move = useMoveRole();
  return {
    update,
    setGrant,
    move,
    failure: [update, setGrant, move].find((write) => write.isError),
    // Every write carries the role's one version. A second write sent before
    // the first answers is refused as a concurrent edit. So one goes at a time,
    // with the version the last one returned.
    busy: update.isPending || setGrant.isPending || move.isPending,
  };
}

export type RoleWrites = ReturnType<typeof useRoleWrites>;

export function RoleDetail({
  role,
  directory,
  units,
  canUpdate,
  canWiden,
  writes,
}: Readonly<{
  role: Role;
  directory: readonly Role[];
  units: readonly ExtensionUnit[];
  canUpdate: boolean;
  /** Whether this reader may turn a right on or widen the row scope. */
  canWiden: boolean;
  writes: RoleWrites;
}>) {
  const t = useT();
  const name = roleLabel(t)(role.key, role.name);
  const archived = Boolean(role.archived_at);
  // An archived role is not editable server-side: it answers as unknown until
  // it is restored, so its controls are read-only here.
  const editable = canUpdate && !archived;
  const { update, setGrant, busy } = writes;
  // The preview is a server read, asked for once the reader opens it.
  const [previewAsked, setPreviewAsked] = useState(false);

  return (
    <div className="roles-detail">
      <p>{archived ? t("roles.archivedNote") : t("roles.detailSub")}</p>
      <SettingList>
        <SettingRow
          label={t("roles.nameLabel")}
          value={name}
          control={
            editable ? (
              <NamePrompt
                trigger={t("roles.rename")}
                title={t("roles.renameTitle")}
                label={t("roles.nameLabel")}
                confirmLabel={t("roles.renameSubmit")}
                pending={busy}
                onSave={(next, done) =>
                  update.mutate(
                    { roleKey: role.key, version: role.version, name: next },
                    { onSuccess: done },
                  )
                }
              />
            ) : null
          }
        />
        <ScopeField
          role={role}
          editable={editable}
          canWiden={canWiden}
          busy={busy}
          onPick={(scope) =>
            update.mutate({
              roleKey: role.key,
              version: role.version,
              rowScope: scope,
            })
          }
        />
        <GrantSection
          groups={grantGroups(directory, units, t)}
          role={role}
          name={name}
          editable={editable}
          canWiden={canWiden}
          busy={busy}
          setGrant={setGrant}
        />
        {!archived && (
          <Disclosure
            summary={t("roles.preview")}
            onToggle={(open) => {
              if (open) setPreviewAsked(true);
            }}
          >
            {previewAsked && (
              <AccessPreviewPanel role={role.key} teamIds={[]} />
            )}
          </Disclosure>
        )}
      </SettingList>
    </div>
  );
}

// Not a hover tip on each name for the raw keys: touch and keyboard never raise it.
function GrantSection({
  groups,
  role,
  name,
  editable,
  canWiden,
  busy,
  setGrant,
}: Readonly<{
  groups: readonly GrantGroup[];
  role: Role;
  name: string;
  editable: boolean;
  canWiden: boolean;
  busy: boolean;
  setGrant: ReturnType<typeof useSetRoleGrant>;
}>) {
  const t = useT();
  const [showKeys, setShowKeys] = useState(false);
  const row = ({ object, name: label, known }: GrantObject): GrantMatrixRow => {
    const grant = grantOf(role, object);
    return {
      key: object,
      name: label,
      code: showKeys || !known ? object : undefined,
      grant,
      cellLabel: (action) =>
        t("extAccess.cell", {
          role: name,
          action: t(`extAccess.action.${action}`),
          object: label,
        }),
      pending:
        setGrant.isPending &&
        setGrant.variables?.roleKey === role.key &&
        setGrant.variables.object === object,
      onChange: (action, next) =>
        setGrant.mutate({
          roleKey: role.key,
          object,
          grant: { ...grant, [action]: next },
          version: role.version,
        }),
    };
  };
  return (
    <SettingRow
      layout="stack"
      label={t("roles.grants")}
      description={
        editable
          ? t(canWiden ? "roles.grantsSub" : "roles.grantsNarrowOnly")
          : undefined
      }
      control={(control) => (
        <div className="settingrow-measure roles-grants">
          <Switch
            label={t("roles.showKeys")}
            checked={showKeys}
            onChange={setShowKeys}
          />
          <GrantMatrix
            rowHeader={t("roles.objectColumn")}
            sections={groups.map((group) => ({
              key: group.key,
              title: group.title,
              rows: group.objects.map(row),
            }))}
            canManage={editable}
            readOnlyReason={t("roles.readOnly")}
            turnOnReason={canWiden ? undefined : t("roles.turnOnAdminOnly")}
            busy={busy}
            name={{ labelledBy: control["aria-labelledby"] }}
          />
        </div>
      )}
    />
  );
}

// Whose records members change: three radios, each with its explanation. A
// reader who may not widen may still narrow, so the wider options are held
// for them, with the reason said once under the choice.
function ScopeField({
  role,
  editable,
  canWiden,
  busy,
  onPick,
}: Readonly<{
  role: Role;
  editable: boolean;
  canWiden: boolean;
  busy: boolean;
  onPick: (scope: RowScope) => void;
}>) {
  const t = useT();
  const held = SCOPES.indexOf(role.row_scope);
  return (
    <SettingRow
      layout="stack"
      label={t("roles.scopeTitle")}
      description={t("roles.scopeSub")}
      control={(control) => (
        <fieldset
          className="roles-scope"
          aria-labelledby={control["aria-labelledby"]}
        >
          {SCOPES.map((scope, at) => (
            <Radio
              key={scope}
              name={`row-scope:${role.key}`}
              checked={role.row_scope === scope}
              disabled={!editable || busy || (!canWiden && at > held)}
              onChange={() => onPick(scope)}
              label={
                <span className="roles-scope-option">
                  <span>{t(`roles.scope.${scope}`)}</span>
                  <span className="t-caption">
                    {t(`roles.scope.${scope}Sub`)}
                  </span>
                </span>
              }
            />
          ))}
          {editable && !canWiden && held < SCOPES.length - 1 && (
            <p className="t-caption">{t("roles.widenAdminOnly")}</p>
          )}
        </fieldset>
      )}
    />
  );
}
