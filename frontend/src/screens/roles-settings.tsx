// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { useCan, useCanWrite, useHoldsAdminRole } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { QueryGate, useMe } from "./common";
import { useExtensions } from "./extensions.queries";
import {
  type Role,
  roleLabel,
  roleOptions,
  roleRefusalOf,
  useCreateRole,
  useRoles,
} from "./roles.queries";
import { RoleDetail } from "./roles-detail";
import "./roles-settings.css";

// Settings → Roles and permissions: every role the company defines, and the
// one open for reading or editing beneath the list. A role is made by copying
// one, so there is no blank role and no "new role" form with sixty switches
// to set before it means anything.

export function RolesSettings() {
  const t = useT();
  const me = useMe();
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
  const [openKey, setOpenKey] = useState<string | null>(null);
  const roles = useRoles(readsRoles, showArchived);
  const units = useExtensions(readsRoles && readsUnits);
  const open = roles.data?.find((role) => role.key === openKey);
  const readOnly = me.isSuccess && !(mayUpdate || mayCreate || mayMove);

  return (
    <div className="roles-stack">
      <Panel
        title={t("roles.title")}
        titleAction={
          mayCreate && isAdmin && roles.data ? (
            <NewRoleAction roles={roles.data} onCreated={setOpenKey} />
          ) : undefined
        }
      >
        <PanelBody>
          <PanelIntro>
            {t("roles.sub")}
            {readOnly && ` ${t("roles.readOnly")}`}
          </PanelIntro>
          <Switch
            label={t("roles.showArchived")}
            hint={t("roles.showArchivedSub")}
            checked={showArchived}
            onChange={setShowArchived}
          />
          <QueryGate query={roles} pendingLabel={t("roles.title")}>
            {(list) => (
              <RoleList
                roles={list}
                openKey={openKey}
                onOpen={(key) => setOpenKey(key)}
              />
            )}
          </QueryGate>
        </PanelBody>
      </Panel>
      {open && (
        <RoleDetail
          key={open.key}
          role={open}
          directory={roles.data ?? []}
          units={units.data ?? []}
          canUpdate={mayUpdate}
          canMove={mayMove}
          canRestore={mayMove && isAdmin}
          canWiden={isAdmin}
        />
      )}
    </div>
  );
}

function RoleList({
  roles,
  openKey,
  onOpen,
}: Readonly<{
  roles: readonly Role[];
  openKey: string | null;
  onOpen: (key: string) => void;
}>) {
  const t = useT();
  const label = roleLabel(t);
  if (roles.length === 0) {
    return <EmptyState>{t("roles.empty")}</EmptyState>;
  }
  return (
    <SettingList>
      {roles.map((role) => {
        const name = label(role.key, role.name);
        return (
          <SettingRow
            key={role.key}
            testId={`role-${role.key}`}
            label={name}
            description={t(`roles.scope.${role.row_scope}Sub`)}
            control={
              <>
                {role.is_system && <Badge>{t("roles.system")}</Badge>}
                {role.archived_at && (
                  <Badge tone="warning">{t("roles.archived")}</Badge>
                )}
                <Button
                  variant={role.key === openKey ? "primary" : undefined}
                  aria-label={t("roles.openNamed", { name })}
                  aria-pressed={role.key === openKey}
                  onClick={() => onOpen(role.key)}
                >
                  {t("roles.open")}
                </Button>
              </>
            }
          />
        );
      })}
    </SettingList>
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
