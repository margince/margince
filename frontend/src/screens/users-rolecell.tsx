import type { components } from "../api/schema";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { QueryLike } from "./common";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  type useAssignableRoles,
} from "./roles.queries";
import type { Role } from "./users-invite-form";

// The member roster's role column: the picker a row draws where the reader may
// change a member's role, and the answer it reads back where they may not.

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

// The role picker, held to the row language's measure so nine of them line up
// at one x rather than each shrinking to its own answer's width.
//
// It draws only where a role is something this reader can CHANGE. The agent
// seat has no role at all and a reader without the grant has a fact rather than
// a control — both read as the row's `value`, decided by the caller, because a
// picker the server would refuse promises something it cannot do.
export function RoleCell({
  member,
  roles,
  pending,
  inFlight,
  onPick,
}: Readonly<{
  member: User;
  roles: readonly AssignableRole[];
  pending: boolean;
  inFlight?: Role;
  onPick: (role: Role) => void;
}>) {
  const t = useT();
  const label = roleLabel(t);
  // `roles` on the member arrives only for a member administrator — which this
  // control always has — and normally holds exactly one key. No key (an
  // unassigned seat) and several keys both leave the select on its
  // placeholder, because neither has one current role to show.
  const heldRoles = member.roles ?? [];
  const currentRole = heldRoles.length === 1 ? (heldRoles[0] ?? "") : "";
  // A member holding SEVERAL roles is the case worth naming: any choice here
  // replaces the whole set, so a neutral "Set role…" would let an admin strip
  // privileges they never saw. The placeholder says what is held instead.
  // plural-rule:allow the two arms name what is held and what to do, which is a
  // state the reader is in rather than two forms of one sentence
  const placeholder =
    heldRoles.length > 1
      ? t("users.rolesHeld", {
          roles: heldRoles
            .map((key) => label(key, nameOf(roles, key)))
            .join(", "),
        })
      : t("users.setRole");
  return (
    // The unset state is the select's PLACEHOLDER, not an option: picking it
    // back would set no role, so it belongs on the closed face and nowhere in
    // the list. It is only ever seen when there is no single role to show.
    //
    // Its own `aria-label` rather than the row's label through `control`'s ARIA:
    // the row names the MEMBER, and a combobox announcing itself as "Ada
    // Active" would leave a reader to guess what picking from it does.
    <Select
      className="settingrow-measure"
      aria-label={t("users.setRoleFor", { name: member.display_name })}
      value={inFlight ?? currentRole}
      placeholder={placeholder}
      disabled={pending}
      onChange={onPick}
      options={roleOptions(t, roles)}
    />
  );
}

// The stored name of a role this reader may assign, for a custom role's label.
function nameOf(
  roles: readonly AssignableRole[],
  key: string,
): string | undefined {
  return roles.find((role) => role.key === key)?.name;
}

// Whether a row draws the role picker. A member holding exactly one role the
// reader may not hand out gets the role as a fact instead: a picker would show
// it as unset, and every choice from it would be one the server refuses.
export function drawsRolePicker(
  member: User,
  roles: readonly AssignableRole[],
  canChangeRole: boolean,
): boolean {
  if (!canChangeRole || member.is_agent) {
    return false;
  }
  const held = member.roles ?? [];
  return held.length !== 1 || roles.some((role) => role.key === held[0]);
}

// The role a row reports rather than offers: the agent seat's, whose authority
// is the passport granting it intersected with the contact that passport names,
// and any member's for a reader who may not change it. `undefined` means the
// row draws a picker instead.
export function roleAnswer(
  member: User,
  roles: readonly AssignableRole[],
  drawsPicker: boolean,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (member.is_agent) {
    return t("users.agentSeatRole");
  }
  if (drawsPicker) {
    return undefined;
  }
  const label = roleLabel(t);
  const held = (member.roles ?? [])
    .map((key) => label(key, nameOf(roles, key)))
    .join(", ");
  // A seat holding no role has nothing to report, and an empty value would
  // draw an empty span where the answer belongs.
  return held === "" ? undefined : held;
}
