// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { type AssignableRole, roleOptions } from "./roles.queries";
import { AccessPreviewPanel } from "./users-access";
import type { Role } from "./users-invite-form";
import { heldRoleNames } from "./users-rolecell";

type User = components["schemas"]["User"];

// A role change behind a dialog rather than a select on the row. The write
// REPLACES the member's whole role set and has no Undo (the roster it would
// restore from is already gone), so the reader sees what is held today, what
// the picked role can reach, and that it replaces everything, before it fires.
export function ChangeRoleDialog({
  member,
  roles,
  open,
  pending,
  error,
  onClose,
  onConfirm,
  returnFocusTo,
}: Readonly<{
  member: User;
  roles: readonly AssignableRole[];
  open: boolean;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: (role: Role) => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const held = member.roles ?? [];
  // Exactly one held role is the only case with a current answer to start on;
  // none or several leave the picker empty, because neither names one role.
  const current = held.length === 1 ? (held[0] ?? "") : "";
  const [picked, setPicked] = useState<string>(current);
  const today = heldRoleNames(member, roles, t);
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("users.roster.changeRoleTitle", { name: member.display_name })}
      confirmLabel={t("users.roster.changeRole")}
      // Picking the role already held, alone, is no change at all.
      confirmDisabled={
        picked === "" || (held.length === 1 && picked === current)
      }
      pending={pending}
      error={error}
      onConfirm={() => onConfirm(picked)}
      returnFocusTo={returnFocusTo}
    >
      <p>
        {t("users.roster.roleToday", {
          role: today === "" ? t("users.roster.noRole") : today,
        })}
      </p>
      <Field label={t("users.roster.newRole")}>
        {(control) => (
          <Select
            {...control}
            value={picked}
            placeholder={t("users.setRole")}
            disabled={pending}
            onChange={setPicked}
            options={roleOptions(t, roles)}
          />
        )}
      </Field>
      {picked !== "" && (
        <AccessPreviewPanel role={picked} teamIds={member.team_ids ?? []} />
      )}
      <Callout tone="warning" title={t("users.roster.replaceTitle")}>
        {t("users.roster.replaceBody", { name: member.display_name })}
      </Callout>
    </ConfirmModal>
  );
}
