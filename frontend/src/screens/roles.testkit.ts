// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { AssignableRole } from "./roles.queries";

// The six seeded roles as GET /users/assignable-roles answers an admin, for
// the tests and stories whose pickers read that list.
export const SEEDED_ASSIGNABLE_ROLES: readonly AssignableRole[] = [
  { key: "admin", name: "Admin", is_system: true },
  { key: "management", name: "Management", is_system: true },
  { key: "manager", name: "Team Lead", is_system: true },
  { key: "ops", name: "Ops / Integrations", is_system: true },
  { key: "read_only", name: "Read-only", is_system: true },
  { key: "rep", name: "User", is_system: true },
];
