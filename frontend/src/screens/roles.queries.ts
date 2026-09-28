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
      const { data, error } = await api.GET("/users/assignable-roles");
      if (error) {
        throwProblem(error);
      }
      return data.roles;
    },
  });
}

// The catalog key each SEEDED role reads under. A seeded role's stored name is
// English and fixed at bootstrap; the catalog carries it in the reader's
// language. `read_only` reads under `role.readOnly`, so the map is written out.
const SEEDED_ROLE_LABEL: Readonly<Record<string, MessageKey>> = {
  admin: "role.admin",
  management: "role.management",
  manager: "role.manager",
  rep: "role.rep",
  read_only: "role.readOnly",
  ops: "role.ops",
};

/**
 * The name a role is shown under: a seeded role in the reader's language, any
 * other under the name its maker gave it, and a key nothing here can name as
 * itself, so the admin still learns what is held.
 */
export const roleLabel =
  (t: ReturnType<typeof useT>) =>
  (key: string, name?: string): string => {
    const seeded = SEEDED_ROLE_LABEL[key];
    if (seeded !== undefined) {
      return t(seeded);
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
      const { data, error } = await api.GET("/roles", {
        params: { query: includeArchived ? { include_archived: true } : {} },
      });
      if (error) {
        throwProblem(error);
      }
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

// What is read off a role, apart from the directory itself.
function refreshReadersOfRoles(client: QueryClient) {
  return Promise.all([
    client.invalidateQueries({ queryKey: ["access-preview"] }),
    client.invalidateQueries({ queryKey: ASSIGNABLE_ROLES_KEY }),
    client.invalidateQueries({ queryKey: ["me"] }),
  ]);
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
      const { data, error } = await api.PATCH("/roles/{key}/objects/{object}", {
        params: {
          path: { key: input.roleKey, object: input.object },
          ...ifMatch(input.version),
        },
        body: input.grant,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The server answers with the whole updated role, so every directory read
    // takes it verbatim: a refetch would repaint the matrix a beat later, and a
    // local merge would invent a grant the server never confirmed.
    onSuccess: (role) => {
      client.setQueriesData<readonly Role[]>({ queryKey: ROLES_KEY }, (roles) =>
        roles?.map((existing) => (existing.key === role.key ? role : existing)),
      );
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
      const { data, error } = await api.PATCH("/roles/{key}", {
        params: {
          path: { key: input.roleKey },
          ...ifMatch(input.version),
        },
        body: { name: input.name, row_scope: input.rowScope },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
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
      const { data, error } = await api.POST("/roles", {
        body: { copy_from: input.copyFrom, name: input.name },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => refreshAfterRoleEdit(client),
  });
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
    onSettled: () => refreshAfterRoleEdit(client),
  });
}
