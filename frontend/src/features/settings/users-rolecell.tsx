import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { CellStack } from "../../design-system/cellstack";
import { InlineChoice } from "../../design-system/inlinechoice";
import type { QueryLike } from "../../design-system/querystates";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { holdsAdminRole } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  type useAssignableRoles,
} from "../../lib/roles.queries";
import type { Role } from "./users-invite-form";
import {
  memberMutationKey,
  offers,
  type User,
  useMemberBusy,
  useMemberRefresh,
} from "./users-members";

// The roster together with the roles its pickers offer, read as one. A row
// drawn before the roles arrive would show a member's role as unset.
export function withRoles(
  members: QueryLike<User[]>,
  assignable: ReturnType<typeof useAssignableRoles>,
  needsRoles: boolean,
): QueryLike<Readonly<{ list: User[]; roles: readonly AssignableRole[] }>> {
  const rolesPending = needsRoles && assignable.isPending;
  const rolesFailed = needsRoles && assignable.isError;
  return {
    isPending: members.isPending || rolesPending,
    isError: members.isError || rolesFailed,
    error: members.error ?? assignable.error,
    data:
      members.data === undefined || rolesPending
        ? undefined
        : { list: members.data, roles: assignable.data ?? [] },
    refetch: () =>
      Promise.all([
        members.refetch(),
        needsRoles ? assignable.refetch() : undefined,
      ]),
  };
}

/** A refused role control, with its own reason unless the card already says one. */
export type RoleRefusal = Readonly<{ reason?: string }>;

export function roleRefusal(
  member: User,
  context: Readonly<{
    roster: readonly User[];
    roles: readonly AssignableRole[];
    canChangeRole: boolean;
    meId: string | undefined;
    t: ReturnType<typeof useT>;
  }>,
): RoleRefusal | null {
  const { roster, roles, t } = context;
  if (!context.canChangeRole) {
    return {};
  }
  const offered = offers(member, "change_role");
  // A delegate's own role is often one they may not hand out, and "outside your
  // access" would misname why their own row is refused.
  if (!offered && member.id === context.meId) {
    return soleActiveAdmin(member, roster)
      ? { reason: t("users.role.lastAdmin") }
      : { reason: t("users.role.own") };
  }
  const held = member.roles ?? [];
  const heldAssignable =
    held.length !== 1 || roles.some((role) => role.key === held[0]);
  if (!heldAssignable) {
    return { reason: t("users.role.outside") };
  }
  if (offered) {
    return null;
  }
  if (soleActiveAdmin(member, roster)) {
    return { reason: t("users.role.lastAdmin") };
  }
  return { reason: t("users.role.outside") };
}

function soleActiveAdmin(member: User, roster: readonly User[]): boolean {
  const activeAdmin = (u: User) =>
    u.status === "active" && holdsAdminRole(u.roles);
  return (
    activeAdmin(member) &&
    !roster.some((other) => other.id !== member.id && activeAdmin(other))
  );
}

export function MemberRole({
  member,
  roles,
  refusal,
}: Readonly<{
  member: User;
  roles: readonly AssignableRole[];
  refusal: RoleRefusal | null;
}>) {
  const t = useT();
  const toast = useToast();
  const refresh = useMemberRefresh();
  const busy = useMemberBusy(member.id);
  const [choosing, setChoosing] = useState(false);
  const setRole = useMutation({
    mutationKey: memberMutationKey(member.id, "role"),
    mutationFn: async ({ id, role }: Readonly<{ id: string; role: Role }>) => {
      unwrap(
        await api.PATCH("/users/{id}/role", {
          params: { path: { id } },
          body: { role },
        }),
      );
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t("users.roleSaved", { name: member.email }));
    },
  });
  if (member.is_agent) {
    return (
      <span className="t-caption">
        {t(
          member.service_extension
            ? "users.serviceMemberRole"
            : "users.agentSeatRole",
        )}
      </span>
    );
  }
  const held = member.roles ?? [];
  const names = held
    .map((key) => roleLabel(t)(key, nameOf(roles, key)))
    .join(", ");
  const value = held.join(" ");
  const several = held.length > 1;
  // The held set is an option of its own, so the open chooser never shows a
  // blank face, and picking it again is no change.
  const options = several
    ? [{ value, label: names }, ...roleOptions(t, roles)]
    : roleOptions(t, roles);
  const reason = refusal?.reason;
  const replaces = several && refusal === null;
  return (
    <CellStack>
      <span className="users-role">
        <InlineChoice
          label={t("users.roleOf", { name: member.display_name })}
          hideLabel
          value={value}
          options={options}
          // Another verb in flight on this member locks the role. This row's own
          // save does not, or the chooser would close on its pick.
          canEdit={refusal === null && (!busy || setRole.isPending)}
          render={() => names}
          onSave={(role) => setRole.mutateAsync({ id: member.id, role })}
          onEditingChange={setChoosing}
        />
      </span>
      {/* Shown at rest: the open list covers whatever stands under the
          chooser. The status says it again as the chooser opens. */}
      {replaces && (
        <>
          <span className="t-caption">{t("users.rolesReplace")}</span>
          <span className="sr-only" role="status">
            {choosing ? t("users.rolesReplace") : ""}
          </span>
        </>
      )}
      {reason && <span className="t-caption">{reason}</span>}
    </CellStack>
  );
}

function nameOf(
  roles: readonly AssignableRole[],
  key: string,
): string | undefined {
  return roles.find((role) => role.key === key)?.name;
}
