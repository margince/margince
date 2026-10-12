import { useMutation } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import { holdsAdminRole } from "../app/capability";
import { ErrorLine } from "../design-system/errorline";
import { Select, type SelectOption } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { type QueryLike, unwrap } from "./common";
import {
  type AssignableRole,
  roleLabel,
  roleOptions,
  type useAssignableRoles,
} from "./roles.queries";
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

/** Why a member's role control is refused: a sentence elsewhere on the page, or its own. */
export type RoleRefusal = Readonly<{ id: string }> | Readonly<{ text: string }>;

export function roleRefusal(
  member: User,
  context: Readonly<{
    roster: readonly User[];
    roles: readonly AssignableRole[];
    canChangeRole: boolean;
    cardReasonId: string;
    meId: string | undefined;
    t: ReturnType<typeof useT>;
  }>,
): RoleRefusal | null {
  const { roster, roles, t } = context;
  if (!context.canChangeRole) {
    return { id: context.cardReasonId };
  }
  const offered = offers(member, "change_role");
  // A delegate's own role is often one they may not hand out, and "outside your
  // access" would misname why their own row is refused.
  if (!offered && member.id === context.meId) {
    return soleActiveAdmin(member, roster)
      ? { text: t("users.role.lastAdmin") }
      : { text: t("users.role.own") };
  }
  const held = member.roles ?? [];
  const heldAssignable =
    held.length !== 1 || roles.some((role) => role.key === held[0]);
  if (!heldAssignable) {
    return { text: t("users.role.outside") };
  }
  if (offered) {
    return null;
  }
  if (soleActiveAdmin(member, roster)) {
    return { text: t("users.role.lastAdmin") };
  }
  return { text: t("users.role.outside") };
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
  const reasonId = useId();
  const errorId = useId();
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
    return <span className="t-caption">{t("users.agentSeatRole")}</span>;
  }
  const described = [
    refusal && ("id" in refusal ? refusal.id : reasonId),
    setRole.isError ? errorId : undefined,
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <span className="users-role">
      <RoleCell
        member={member}
        roles={roles}
        pending={busy}
        refused={refusal !== null}
        describedBy={described || undefined}
        // A failed change falls back to the held role, so re-picking the same
        // target still fires onChange.
        inFlight={setRole.isPending ? setRole.variables.role : undefined}
        onPick={(role) => setRole.mutate({ id: member.id, role })}
      />
      {refusal && "text" in refusal && (
        <span id={reasonId} className="t-caption">
          {refusal.text}
        </span>
      )}
      <ErrorLine id={errorId} error={setRole.error} />
    </span>
  );
}

// Fixed width, so every row's picker starts and ends at one x.
export function RoleCell({
  member,
  roles,
  pending,
  refused = false,
  describedBy,
  inFlight,
  onPick,
}: Readonly<{
  member: User;
  roles: readonly AssignableRole[];
  pending: boolean;
  refused?: boolean;
  describedBy?: string;
  inFlight?: Role;
  onPick: (role: Role) => void;
}>) {
  const t = useT();
  const label = roleLabel(t);
  const heldRoles = member.roles ?? [];
  const currentRole = heldRoles.length === 1 ? (heldRoles[0] ?? "") : "";
  // Any choice replaces the whole set, so several held roles are named rather
  // than hidden behind a neutral "Set role…".
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
    <Select
      className="users-role-select"
      aria-label={t("users.setRoleFor", { name: member.display_name })}
      aria-describedby={describedBy}
      value={inFlight ?? currentRole}
      placeholder={placeholder}
      disabled={pending || refused}
      onChange={onPick}
      options={withHeld(roleOptions(t, roles), currentRole, label)}
    />
  );
}

// A held role this reader may not hand out still names itself on the face.
function withHeld(
  options: SelectOption[],
  held: string,
  label: (key: string) => string,
): SelectOption[] {
  if (held === "" || options.some((option) => option.value === held)) {
    return options;
  }
  return [...options, { value: held, label: label(held), disabled: true }];
}

function nameOf(
  roles: readonly AssignableRole[],
  key: string,
): string | undefined {
  return roles.find((role) => role.key === key)?.name;
}
