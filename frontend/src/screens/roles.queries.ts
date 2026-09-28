// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { SelectOption } from "../design-system/select";
import type { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";

// The roles a workspace defines, as the pickers and the role editor read them.
// A role is a row the server holds, not a list compiled into the client: an
// operator makes new ones, and a picker holding a fixed list would refuse to
// offer them.

export type AssignableRole = components["schemas"]["AssignableRole"];

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
