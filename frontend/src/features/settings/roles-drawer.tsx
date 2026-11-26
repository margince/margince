// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useRef, useState } from "react";
import { Badge, Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { ExtensionUnit } from "../../lib/extensions.queries";
import {
  type Role,
  roleLabel,
  roleRefusalOf,
  useAssignableRoles,
} from "../../lib/roles.queries";
import {
  type ObjectVerbs,
  RoleDetail,
  type RoleWrites,
  useRoleWrites,
} from "./roles-detail";

/** A role's kind, and whether it is archived: the row and the drawer say both. */
export function RoleBadges({ role }: Readonly<{ role: Role }>) {
  const t = useT();
  return (
    <>
      {role.is_system ? (
        <Badge>{t("roles.system")}</Badge>
      ) : (
        <Badge tone="accent">{t("roles.custom")}</Badge>
      )}
      {role.archived_at && <Badge tone="warning">{t("roles.archived")}</Badge>}
    </>
  );
}

export function RoleDrawer({
  role,
  open,
  onClose,
  returnFocusTo,
  directory,
  units,
  verbs,
  canUpdate,
  canMove,
  canRestore,
  canWiden,
}: Readonly<{
  role: Role;
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
  directory: readonly Role[];
  units: readonly ExtensionUnit[];
  verbs: ObjectVerbs;
  canUpdate: boolean;
  canMove: boolean;
  canRestore: boolean;
  canWiden: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const titleId = useId();
  const body = useRef<HTMLDivElement | null>(null);
  const writes = useRoleWrites();
  const [confirmingRestore, setConfirmingRestore] = useState(false);
  const current = newest(role, writes);
  const name = roleLabel(t)(current.key, current.name);
  const moveVerb = moveOf(current, canMove, canRestore);
  // Only the assignable roles say whether a role captures; a role absent there,
  // such as an archived one, says nothing rather than guess.
  const assignable = useAssignableRoles(open);
  const capturesMail = assignable.data?.find(
    (entry) => entry.key === current.key,
  )?.captures_mail;
  return (
    <Modal
      open={open}
      // A write closed over is still sent, and its refusal would land nowhere.
      onClose={() => {
        if (!writes.busy) onClose();
      }}
      closeDisabled={writes.busy}
      labelledBy={titleId}
      intent="drawer-reading"
      returnFocusTo={returnFocusTo}
      // The box, not a control: the head's first stop archives at once, and a
      // read-only body's first stop sits a grid's length down.
      initialFocusTo={() =>
        body.current?.closest<HTMLElement>("[role=dialog]") ?? null
      }
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {name}
        </Heading>
        <div className="roles-drawer-sub">
          <div className="roles-drawer-facts">
            <span className="roles-drawer-badges">
              <RoleBadges role={current} />
            </span>
            {current.is_system && !current.archived_at && canUpdate && (
              <Button
                disabled={writes.busy}
                onClick={() => setConfirmingRestore(true)}
              >
                {t("roles.restoreGrants")}
              </Button>
            )}
            {moveVerb && (
              <Button
                disabled={writes.busy}
                onClick={() =>
                  writes.move.mutate(
                    { roleKey: current.key, to: moveVerb },
                    {
                      onSuccess: () =>
                        toast.show(
                          t(
                            moveVerb === "archive"
                              ? "roles.archivedToast"
                              : "roles.restoredToast",
                            { name },
                          ),
                        ),
                    },
                  )
                }
              >
                {t(moveVerb === "archive" ? "roles.archive" : "roles.restore")}
              </Button>
            )}
          </div>
          {capturesMail === false && (
            <Callout
              tone="info"
              kind="standing"
              title={t("roles.cannotCapture")}
            />
          )}
          {/* In the head, so a refusal stays in view however far the grid scrolled. */}
          {writes.failure && (
            <Callout tone="danger" kind="outcome" title={t("roles.notSaved")}>
              {roleRefusalOf(writes.failure.error, t)}
            </Callout>
          )}
        </div>
      </DrawerHead>
      <ConfirmModal
        open={confirmingRestore}
        onClose={() => setConfirmingRestore(false)}
        title={t("roles.restoreGrantsTitle", { name })}
        confirmLabel={t("roles.restoreGrants")}
        confirmVariant="danger"
        pending={writes.restoreGrants.isPending}
        onConfirm={() =>
          writes.restoreGrants.mutate(
            { roleKey: current.key, version: current.version },
            {
              onSuccess: () => {
                setConfirmingRestore(false);
                toast.show(t("roles.restoredGrantsToast", { name }));
              },
              onError: () => setConfirmingRestore(false),
            },
          )
        }
      >
        <p>{t("roles.restoreGrantsBody")}</p>
      </ConfirmModal>
      <DrawerBody ref={body} className="roles-drawer-body">
        <RoleDetail
          role={current}
          directory={directory}
          units={units}
          verbs={verbs}
          canUpdate={canUpdate}
          canWiden={canWiden}
          writes={writes}
        />
      </DrawerBody>
    </Modal>
  );
}

// The role as its last answered write left it, when newer than the list's
// copy. Archiving drops a role from a list that hides archived ones.
function newest(role: Role, writes: RoleWrites): Role {
  return [
    writes.update.data,
    writes.setGrant.data,
    writes.move.data,
    writes.restoreGrants.data,
  ].reduce<Role>(
    (best, answer) =>
      answer?.key === best.key && answer.version > best.version ? answer : best,
    role,
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
