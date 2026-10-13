import type { components } from "../api/schema";
import type { useT } from "../i18n";
import type { QueryLike } from "./common";
import {
  type AssignableRole,
  roleLabel,
  type useAssignableRoles,
} from "./roles.queries";

// The member roster's role column: what each row reads back, and whether it
// offers the change that the dialog in users-rolechange.tsx makes.

type User = components["schemas"]["User"];

// The roster together with the roles its pickers offer, read as one. A row
// drawn before the roles arrive would show a member's role as unset, and a
// reader who changes no roles never asks for them.
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

// The stored name of a role this reader may assign, for a custom role's label.
function nameOf(
  roles: readonly AssignableRole[],
  key: string,
): string | undefined {
  return roles.find((role) => role.key === key)?.name;
}

// Whether a row offers "Change role". The server's `change_role` says a change
// is possible at all; a member holding exactly one role the reader may not hand
// out still gets none, because every choice the dialog offered would replace a
// role this reader cannot give back.
export function offersRoleChange(
  member: User,
  roles: readonly AssignableRole[],
  offered: boolean,
): boolean {
  if (!offered || member.is_agent) {
    return false;
  }
  const held = member.roles ?? [];
  return held.length !== 1 || roles.some((role) => role.key === held[0]);
}

// The roles a member holds, under their labels and joined; "" for none.
export function heldRoleNames(
  member: User,
  roles: readonly AssignableRole[],
  t: ReturnType<typeof useT>,
): string {
  const label = roleLabel(t);
  return (member.roles ?? [])
    .map((key) => label(key, nameOf(roles, key)))
    .join(", ");
}

// What the role column reads on every row, as text, so the column has one shape
// whoever may change what. The caption says why a reader who changes roles
// cannot change THIS one; the server sends no reason, so it says no more than
// that.
export function roleReading(
  member: User,
  roles: readonly AssignableRole[],
  locked: boolean,
  t: ReturnType<typeof useT>,
): Readonly<{ role: string; caption?: string }> {
  if (member.is_agent) {
    return { role: t("users.roster.agentRole") };
  }
  const held = heldRoleNames(member, roles, t);
  return {
    role: held === "" ? t("users.roster.noRole") : held,
    caption: locked ? t("users.roster.roleLocked") : undefined,
  };
}
