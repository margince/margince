// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useRef } from "react";
import { Badge, Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { ExtensionUnit } from "../../lib/extensions.queries";
import { type Role, roleLabel, roleRefusalOf } from "../../lib/roles.queries";
import { RoleDetail, type RoleWrites, useRoleWrites } from "./roles-detail";

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
  const current = newest(role, writes);
  const name = roleLabel(t)(current.key, current.name);
  const moveVerb = moveOf(current, canMove, canRestore);
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
          {/* In the head, so a refusal stays in view however far the grid scrolled. */}
          {writes.failure && (
            <Callout tone="danger" kind="outcome" title={t("roles.notSaved")}>
              {roleRefusalOf(writes.failure.error, t)}
            </Callout>
          )}
        </div>
      </DrawerHead>
      <DrawerBody ref={body} className="roles-drawer-body">
        <RoleDetail
          role={current}
          directory={directory}
          units={units}
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
