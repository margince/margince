// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useRosterWalk } from "../../components/roster";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { Heading } from "../../design-system/heading";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelRow,
} from "../../design-system/panel";
import { useLastSubject } from "../../design-system/presence";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite, useHoldsAdminRole } from "../../lib/capability";
import { useExtensions } from "../../lib/extensions.queries";
import {
  type Role,
  roleLabel,
  roleOptions,
  roleRefusalOf,
  useCreateRole,
  useRoles,
} from "../../lib/roles.queries";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { RoleBadges, RoleDrawer } from "./roles-drawer";
import "./roles-settings.css";

// Settings → Roles and permissions: every role the company defines, and the
// one open in a drawer. A role is made by copying one, so there is no blank
// role and no form with sixty switches to set before it means anything.

// The open role rides the address, so a reload or a shared link reopens it.
const ROLE_PARAM = "role";

export function RolesSettings() {
  const t = useT();
  // Each hook on its own line: `&&` short-circuits, and the hooks a render
  // calls must not depend on an answer.
  const readsRoles = useCan("role_admin", "read");
  const readsUnits = useCan("extension_access", "read");
  const mayUpdate = useCanWrite("role_admin", "update");
  const mayCreate = useCanWrite("role_admin", "create");
  const mayMove = useCanWrite("role_admin", "delete");
  // Creating and restoring a role are an admin's alone server-side
  // (refuseWideningUnlessAdmin), with no grant that stands for them.
  const isAdmin = useHoldsAdminRole();
  const [showArchived, setShowArchived] = useState(false);
  const [params] = useUrlParams();
  const openKey = params.get(ROLE_PARAM) ?? null;
  const rowIds = useId();
  const rowId = (key: string) => `${rowIds}-${key}`;
  const openRole = (key: string | undefined) => replaceDial(ROLE_PARAM, key);
  const readsMembers = useCan("user_admin", "read");
  const roles = useRoles(readsRoles, showArchived);
  const linked = useLinkedRole(roles, openKey, readsRoles && !showArchived);
  const roster = useRosterWalk("user", readsRoles && readsMembers);
  const units = useExtensions(readsRoles && readsUnits);
  const found: Role | null =
    roles.data?.find((role) => role.key === openKey) ?? linked;
  // Kept while the drawer leaves. Kept too when a role archived in it drops out
  // of a list that hides archived roles, so Restore stays one press away.
  const shown = useLastSubject(found);

  return (
    <>
      <Panel
        title={t("roles.title")}
        titleAction={
          mayCreate && isAdmin && roles.data ? (
            <NewRoleAction roles={roles.data} onCreated={openRole} />
          ) : undefined
        }
        actions={
          // Kept while archived roles are asked for, so a failed read of them
          // can still be turned back off.
          (roles.data !== undefined || showArchived) && (
            <Switch
              label={t("roles.showArchived")}
              hint={t("roles.showArchivedSub")}
              checked={showArchived}
              onChange={setShowArchived}
            />
          )
        }
      >
        <PanelBody>
          <PanelIntro>{t("roles.sub")}</PanelIntro>
        </PanelBody>
        <QueryGate query={roles} pendingLabel={t("roles.title")}>
          {(list) => (
            <RoleList
              roles={list}
              members={memberCounts(roster.data)}
              openKey={openKey}
              rowId={rowId}
              onOpen={openRole}
            />
          )}
        </QueryGate>
      </Panel>
      {shown && (
        <RoleDrawer
          key={shown.key}
          role={shown}
          open={shown.key === openKey}
          onClose={() => openRole(undefined)}
          returnFocusTo={() => document.getElementById(rowId(shown.key))}
          directory={roles.data ?? []}
          units={units.data ?? []}
          canUpdate={mayUpdate}
          canMove={mayMove}
          canRestore={mayMove && isAdmin}
          canWiden={isAdmin}
        />
      )}
    </>
  );
}

// A link may name an archived role while the list hides archived ones, so
// that role is read with them. A key no read lists leaves the address.
function useLinkedRole(
  roles: ReturnType<typeof useRoles>,
  openKey: string | null,
  mayLookWider: boolean,
): Role | null {
  const listed = roles.data?.some((role) => role.key === openKey) ?? true;
  const wider = useRoles(mayLookWider && openKey !== null && !listed, true);
  const searched = mayLookWider ? wider : roles;
  const linked = wider.data?.find((role) => role.key === openKey) ?? null;
  const unknown =
    openKey !== null &&
    !listed &&
    linked === null &&
    !roles.isFetching &&
    searched.isSuccess &&
    !searched.isFetching;
  useEffect(() => {
    if (unknown) {
      replaceDial(ROLE_PARAM, undefined);
    }
  }, [unknown]);
  return linked;
}

type RosterWalk = NonNullable<ReturnType<typeof useRosterWalk>["data"]>;

// Undefined when the roster cannot say. Role keys ride only for a member
// admin, and a walk that stopped short would count low while reading as whole.
export function memberCounts(
  roster: RosterWalk | undefined,
): ReadonlyMap<string, number> | undefined {
  if (!roster || roster.partial) {
    return undefined;
  }
  const counts = new Map<string, number>();
  for (const entry of roster.entries) {
    if (!("email" in entry)) {
      continue;
    }
    if (entry.roles === undefined) {
      return undefined;
    }
    for (const key of entry.roles) {
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  return counts;
}

// Custom roles first: they are the ones an admin made and comes back to edit.
function customFirst(roles: readonly Role[]): Role[] {
  return [...roles].sort((a, b) => Number(a.is_system) - Number(b.is_system));
}

function RoleList({
  roles,
  members,
  openKey,
  rowId,
  onOpen,
}: Readonly<{
  roles: readonly Role[];
  members: ReadonlyMap<string, number> | undefined;
  openKey: string | null;
  rowId: (key: string) => string;
  onOpen: (key: string) => void;
}>) {
  const t = useT();
  if (roles.length === 0) {
    return (
      <PanelBody>
        <EmptyState>{t("roles.empty")}</EmptyState>
      </PanelBody>
    );
  }
  return (
    <>
      {customFirst(roles).map((role) => (
        <RoleRow
          key={role.key}
          role={role}
          members={members?.get(role.key) ?? (members ? 0 : undefined)}
          id={rowId(role.key)}
          open={role.key === openKey}
          onOpen={() => onOpen(role.key)}
        />
      ))}
    </>
  );
}

function RoleRow({
  role,
  members,
  id: rowId,
  open,
  onOpen,
}: Readonly<{
  role: Role;
  members: number | undefined;
  id: string;
  open: boolean;
  onOpen: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const id = useId();
  return (
    <PanelRow interactive record className="roles-row">
      <button
        type="button"
        id={rowId}
        className="roles-row-press"
        data-testid={`role-${role.key}`}
        aria-current={open ? "true" : undefined}
        aria-labelledby={`${id}-name`}
        aria-describedby={`${id}-scope ${id}-facts`}
        onClick={onOpen}
      >
        <CellStack>
          <span className="t-name" id={`${id}-name`}>
            {roleLabel(t)(role.key, role.name)}
          </span>
          <span className="t-caption" id={`${id}-scope`}>
            {t(`roles.scope.${role.row_scope}Sub`)}
          </span>
        </CellStack>
        <span className="roles-row-facts" id={`${id}-facts`}>
          {members !== undefined && (
            <span className="t-caption t-num">
              {plural("roles.members", members, {
                count: formatNumber(members, locale),
              })}
            </span>
          )}
          <RoleBadges role={role} />
        </span>
        <ChevronRight className="roles-row-go" aria-hidden />
      </button>
    </PanelRow>
  );
}

// A new role is a copy: the source and a name, committed together. The dialog
// closes on the write that landed, never before it, so a refused name stays
// where the admin typed it.
function NewRoleAction({
  roles,
  onCreated,
}: Readonly<{ roles: readonly Role[]; onCreated: (key: string) => void }>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const live = roles.filter((role) => !role.archived_at);
  const [copyFrom, setCopyFrom] = useState(live[0]?.key ?? "");
  const [name, setName] = useState("");
  const create = useCreateRole();
  const ready = copyFrom !== "" && name.trim() !== "" && !create.isPending;
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("roles.new")}</Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent="form"
      >
        <Heading size="large" className="t-h3 modal-title" id={titleId}>
          {t("roles.newTitle")}
        </Heading>
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (!ready) {
              return;
            }
            create.mutate(
              { copyFrom, name: name.trim() },
              {
                onSuccess: (role) => {
                  setOpen(false);
                  setName("");
                  onCreated(role.key);
                  toast.show(t("roles.created", { name: role.name }));
                },
              },
            );
          }}
        >
          <Field label={t("roles.newFrom")} hint={t("roles.newFromHint")}>
            {(control) => (
              <Select
                {...control}
                value={copyFrom}
                onChange={setCopyFrom}
                options={roleOptions(t, live)}
              />
            )}
          </Field>
          <Field label={t("roles.newName")} required>
            {(control) => (
              <TextInput
                {...control}
                value={name}
                maxLength={120}
                disabled={create.isPending}
                onChange={(event) => setName(event.target.value)}
              />
            )}
          </Field>
          {create.isError && (
            <Callout tone="danger" kind="outcome" title={t("roles.notCreated")}>
              {roleRefusalOf(create.error, t)}
            </Callout>
          )}
        </form>
        <div className="actions">
          <Button
            type="submit"
            form={formId}
            variant="primary"
            disabled={!ready}
          >
            {t("roles.newSubmit")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
