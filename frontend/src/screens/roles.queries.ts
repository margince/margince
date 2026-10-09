// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import type { SelectOption } from "../design-system/select";
import type { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  isVersionSkewOf,
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "./common";

// The roles a workspace defines, as the pickers and the role editor read them.
// A role is a row the server holds, not a list compiled into the client: an
// operator makes new ones, and a picker holding a fixed list would refuse to
// offer them.

export type AssignableRole = components["schemas"]["AssignableRole"];
export type Role = components["schemas"]["Role"];
export type ObjectGrant = components["schemas"]["RbacObjectGrant"];
export type RowScope = Role["row_scope"];

/** The query key every read of the assignable roles shares. */
export const ASSIGNABLE_ROLES_KEY = ["assignable-roles"] as const;

/**
 * The live roles this reader may hand out, from the same ceiling the invite and
 * the role change run. Asked only by a reader who invites or re-roles members;
 * any other reader's request can only be refused.
 */
export function useAssignableRoles(enabled: boolean) {
  return useQuery({
    queryKey: ASSIGNABLE_ROLES_KEY,
    enabled,
    queryFn: async (): Promise<AssignableRole[]> => {
      const data = unwrap(await api.GET("/users/assignable-roles"));
      return data.roles;
    },
  });
}

// Each SEEDED role's catalog key and the name bootstrap stores for it. The
// stored name is English; the catalog carries it in the reader's language —
// but only while the name is still the one seeded. An operator who renamed
// the role chose the words the company reads, and those win.
// Held to identity's systemRoles table by backend/gates/frontendrolekeys_test.go.
const SEEDED_ROLES: Readonly<
  Record<string, Readonly<{ label: MessageKey; name: string }>>
> = {
  admin: { label: "role.admin", name: "Admin" },
  management: { label: "role.management", name: "Management" },
  manager: { label: "role.manager", name: "Team Lead" },
  rep: { label: "role.rep", name: "User" },
  read_only: { label: "role.readOnly", name: "Read-only" },
  ops: { label: "role.ops", name: "Ops / Integrations" },
};

/**
 * The name a role is shown under: a seeded role still under its seeded name
 * in the reader's language, a renamed or made role under its stored name, and
 * a key nothing here can name as itself, so the admin still learns what is
 * held. With no stored name to hand, a seeded key reads translated.
 */
export const roleLabel =
  (t: ReturnType<typeof useT>) =>
  (key: string, name?: string): string => {
    const seeded = SEEDED_ROLES[key];
    if (seeded !== undefined && (name === undefined || name === seeded.name)) {
      return t(seeded.label);
    }
    return name ?? key;
  };

/** Roles as pickable options, each under its label. */
export function roleOptions(
  t: ReturnType<typeof useT>,
  roles: readonly Readonly<{ key: string; name: string }>[],
): SelectOption[] {
  const label = roleLabel(t);
  return roles.map((role) => ({
    value: role.key,
    label: label(role.key, role.name),
  }));
}

/** The prefix every read of the role directory shares. */
export const ROLES_KEY = ["roles"] as const;

/** The key of one directory read: the live roles, or archived ones too. */
export function rolesKey(includeArchived: boolean) {
  return [...ROLES_KEY, includeArchived ? "with-archived" : "live"] as const;
}

/**
 * The role directory the role editor and the extension grants read. Gated on
 * `role_admin:read` server-side; a reader without it is refused, so it is not
 * asked.
 */
export function useRoles(enabled: boolean, includeArchived = false) {
  return useQuery({
    queryKey: rolesKey(includeArchived),
    enabled,
    queryFn: async (): Promise<readonly Role[]> => {
      // The flag rides only when asked: the live read is the plain URL.
      const data = unwrap(
        await api.GET("/roles", {
          params: { query: includeArchived ? { include_archived: true } : {} },
        }),
      );
      return data.roles;
    },
  });
}

/**
 * Everything a role edit can change the answer of: the directory, and what is
 * read off a role — the access preview, the names the pickers offer, and the
 * reader's own grants when the role edited is one they hold.
 */
export function refreshAfterRoleEdit(client: QueryClient) {
  return Promise.all([
    client.invalidateQueries({ queryKey: ROLES_KEY }),
    refreshReadersOfRoles(client),
  ]);
}

// What is read off a role, apart from the directory itself. The two rosters
// are among them: each member's allowed actions follow the roles they and the
// reader hold.
function refreshReadersOfRoles(client: QueryClient) {
  return Promise.all([
    client.invalidateQueries({ queryKey: ["access-preview"] }),
    client.invalidateQueries({ queryKey: ASSIGNABLE_ROLES_KEY }),
    client.invalidateQueries({ queryKey: ["me"] }),
    client.invalidateQueries({ queryKey: ["users-admin"] }),
    client.invalidateQueries({ queryKey: ["users"] }),
  ]);
}

// Puts the role a write answered with into every directory read.
function replaceRole(client: QueryClient, role: Role) {
  client.setQueriesData<readonly Role[]>({ queryKey: ROLES_KEY }, (roles) =>
    roles?.map((existing) => (existing.key === role.key ? role : existing)),
  );
}

/**
 * Sets one role's whole grant on one object. The PATCH takes the whole grant
 * rather than a delta, and `If-Match` carries the version the reader was
 * looking at, so the server compares against what the operator saw. A refused
 * write is never replayed against a fresh version: that would apply an intent
 * formed against grants the operator had not seen.
 */
export function useSetRoleGrant() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      roleKey: string;
      object: string;
      grant: ObjectGrant;
      version: number;
    }): Promise<Role> => {
      return unwrap(
        await api.PATCH("/roles/{key}/objects/{object}", {
          params: {
            path: { key: input.roleKey, object: input.object },
            ...ifMatch(input.version),
          },
          body: input.grant,
        }),
      );
    },
    // The server answers with the whole updated role, so every directory read
    // takes it verbatim: a refetch would repaint the matrix a beat later, and a
    // local merge would invent a grant the server never confirmed.
    onSuccess: (role) => {
      replaceRole(client, role);
      void refreshReadersOfRoles(client);
    },
    // Another admin's concurrent change is the likeliest refusal, so re-read
    // rather than trust the snapshot the failure was computed against.
    onError: () => {
      void client.invalidateQueries({ queryKey: ROLES_KEY });
    },
  });
}

// The role editor's refusals, by the code the server names them with.
const ROLE_REFUSAL: Readonly<Record<string, MessageKey>> = {
  widening_requires_admin: "roles.refusal.widening",
  role_in_use: "roles.refusal.inUse",
  archived_role_held: "roles.refusal.archivedHeld",
  system_role: "roles.refusal.system",
  admin_role_floor: "roles.refusal.adminFloor",
  role_name_taken: "roles.refusal.nameTaken",
};

/**
 * A refused role edit in plain words: the editor's own refusals and a
 * concurrent edit in the catalog's words, anything else in the server's.
 */
export function roleRefusalOf(
  error: unknown,
  t: ReturnType<typeof useT>,
): string {
  if (isVersionSkewOf(error)) {
    return t("roles.refusal.versionSkew");
  }
  const key = ROLE_REFUSAL[problemCodeOf(error) ?? ""];
  return key === undefined ? problemMessageOf(error, t) : t(key);
}

/**
 * Renames a role or moves its row scope. `If-Match` carries the version the
 * reader saw, so a concurrent edit answers `version_skew` rather than being
 * overwritten.
 */
export function useUpdateRole() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      roleKey: string;
      version: number;
      name?: string;
      rowScope?: RowScope;
    }): Promise<Role> => {
      return unwrap(
        await api.PATCH("/roles/{key}", {
          params: {
            path: { key: input.roleKey },
            ...ifMatch(input.version),
          },
          body: { name: input.name, row_scope: input.rowScope },
        }),
      );
    },
    // The answer carries the role's new version; the next write on it must
    // send that one, so the directory takes it before the refetch lands.
    onSuccess: (role) => replaceRole(client, role),
    onSettled: () => refreshAfterRoleEdit(client),
  });
}

/** Makes a role as a copy of a live one, under a new name. */
export function useCreateRole() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      copyFrom: string;
      name: string;
    }): Promise<Role> => {
      return unwrap(
        await api.POST("/roles", {
          body: { copy_from: input.copyFrom, name: input.name },
        }),
      );
    },
    // The new role lands in the directory before the refetch, so the editor can
    // open it even when that refetch fails and the old list is kept.
    onSuccess: (role) => {
      addRole(client, role);
      return refreshAfterRoleEdit(client);
    },
  });
}

// Puts a newly made role into every directory read, in the server's key order:
// keys are ASCII and the server sorts them bytewise, not by any locale.
function addRole(client: QueryClient, role: Role) {
  client.setQueriesData<readonly Role[]>({ queryKey: ROLES_KEY }, (roles) =>
    roles === undefined || roles.some((existing) => existing.key === role.key)
      ? roles
      : [...roles, role].sort((left, right) =>
          left.key < right.key ? -1 : Number(left.key > right.key),
        ),
  );
}

/** Archives a custom role, or restores an archived one. */
export function useMoveRole() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      roleKey: string;
      to: "archive" | "restore";
    }): Promise<Role> => {
      const params = { params: { path: { key: input.roleKey } } };
      const { data, error } =
        input.to === "archive"
          ? await api.POST("/roles/{key}/archive", params)
          : await api.POST("/roles/{key}/restore", params);
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The answer carries the role's new version; the next write on it must
    // send that one, so the directory takes it before the refetch lands.
    onSuccess: (role) => replaceRole(client, role),
    onSettled: () => refreshAfterRoleEdit(client),
  });
}
