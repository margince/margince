// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure, Radio } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { NamePrompt } from "../design-system/nameprompt";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import type { ExtensionUnit } from "./extensions.queries";
import { GrantMatrix, type GrantMatrixRow, grantOf } from "./grantmatrix";
import {
  type Role,
  type RowScope,
  roleLabel,
  roleRefusalOf,
  useMoveRole,
  useSetRoleGrant,
  useUpdateRole,
} from "./roles.queries";
import { AccessPreviewPanel } from "./users-access";

// One role, open beneath the list: its name, whose records it reaches, what it
// may do with each object, what a member holding it would see, and the verb
// that takes it out of use. Every control writes as it is used, carrying the
// version the reader was looking at.

const SCOPES: readonly RowScope[] = ["own", "team", "all"];

/**
 * The objects a grid row is drawn for, core first and extension objects apart.
 * Read off the directory rather than a list compiled into the client: a seeded
 * role names every core object, and an object a removed unit left behind still
 * shows, so an operator can find and clear its grant.
 */
export function grantObjects(
  directory: readonly Role[],
  units: readonly ExtensionUnit[],
): { core: string[]; extensions: string[] } {
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
  const sorted = [...all].sort(stable);
  return {
    core: sorted.filter((object) => !object.startsWith("ext_")),
    extensions: sorted.filter((object) => object.startsWith("ext_")),
  };
}

export function RoleDetail({
  role,
  directory,
  units,
  canUpdate,
  canMove,
  canRestore,
  canWiden,
}: Readonly<{
  role: Role;
  directory: readonly Role[];
  units: readonly ExtensionUnit[];
  canUpdate: boolean;
  canMove: boolean;
  canRestore: boolean;
  /** Whether this reader may turn a right on or widen the row scope. */
  canWiden: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const name = roleLabel(t)(role.key, role.name);
  const archived = Boolean(role.archived_at);
  // An archived role is not editable server-side: it answers as unknown until
  // it is restored, so its controls are read-only here.
  const editable = canUpdate && !archived;
  const update = useUpdateRole();
  const setGrant = useSetRoleGrant();
  const move = useMoveRole();
  const failure = [update, setGrant, move].find((write) => write.isError);
  // Every write on a role carries the role's one version, so a second write
  // sent before the first answers would be refused as a concurrent edit. One
  // write at a time: the next is sent with the version the last one returned.
  const busy = update.isPending || setGrant.isPending || move.isPending;
  const objects = grantObjects(directory, units);
  const moveVerb = moveOf(role, canMove, canRestore);

  return (
    <Panel
      title={name}
      actions={
        moveVerb && (
          <MoveAction
            to={moveVerb}
            pending={busy}
            onMove={(to) =>
              move.mutate(
                { roleKey: role.key, to },
                {
                  onSuccess: () =>
                    toast.show(
                      t(
                        to === "archive"
                          ? "roles.archivedToast"
                          : "roles.restoredToast",
                        { name },
                      ),
                    ),
                },
              )
            }
          />
        )
      }
    >
      <PanelBody>
        <PanelIntro>
          {archived ? t("roles.archivedNote") : t("roles.detailSub")}
        </PanelIntro>
        {failure && (
          <Callout tone="danger" kind="outcome" title={t("roles.notSaved")}>
            {roleRefusalOf(failure.error, t)}
          </Callout>
        )}
      </PanelBody>
      <SettingList bleed="settings">
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
          title={t("roles.grantsCore")}
          objects={objects.core}
          role={role}
          name={name}
          editable={editable}
          canWiden={canWiden}
          busy={busy}
          setGrant={setGrant}
        />
        {objects.extensions.length > 0 && (
          <GrantSection
            title={t("roles.grantsExtensions")}
            objects={objects.extensions}
            role={role}
            name={name}
            editable={editable}
            canWiden={canWiden}
            busy={busy}
            setGrant={setGrant}
          />
        )}
        {!archived && (
          <Disclosure summary={t("roles.preview")}>
            <AccessPreviewPanel role={role.key} teamIds={[]} />
          </Disclosure>
        )}
      </SettingList>
    </Panel>
  );
}

// The verb that takes a role out of use or brings it back. A built-in role is
// never archived, so it is offered nothing.
function moveOf(
  role: Role,
  canMove: boolean,
  canRestore: boolean,
): "archive" | "restore" | null {
  if (role.archived_at) {
    return canRestore ? "restore" : null;
  }
  return role.is_system || !canMove ? null : "archive";
}

function MoveAction({
  to,
  pending,
  onMove,
}: Readonly<{
  to: "archive" | "restore";
  pending: boolean;
  onMove: (to: "archive" | "restore") => void;
}>) {
  const t = useT();
  return (
    <Button disabled={pending} onClick={() => onMove(to)}>
      {t(to === "archive" ? "roles.archive" : "roles.restore")}
    </Button>
  );
}

// One block of the grid: a row per object over this one role.
function GrantSection({
  title,
  objects,
  role,
  name,
  editable,
  canWiden,
  busy,
  setGrant,
}: Readonly<{
  title: string;
  objects: readonly string[];
  role: Role;
  name: string;
  editable: boolean;
  canWiden: boolean;
  busy: boolean;
  setGrant: ReturnType<typeof useSetRoleGrant>;
}>) {
  const t = useT();
  const rows = objects.map((object): GrantMatrixRow => {
    const grant = grantOf(role, object);
    return {
      key: object,
      name: object,
      grant,
      cellLabel: (action) =>
        t("extAccess.cell", {
          role: name,
          action: t(`extAccess.action.${action}`),
          object,
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
  });
  return (
    <SettingRow
      layout="stack"
      label={title}
      description={
        editable
          ? t(canWiden ? "roles.grantsSub" : "roles.grantsNarrowOnly")
          : undefined
      }
      control={(control) => (
        <div className="settingrow-measure">
          <GrantMatrix
            rowHeader={t("roles.objectColumn")}
            rows={rows}
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
