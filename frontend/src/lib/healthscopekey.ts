// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export function healthScope(
  me: components["schemas"]["MeResponse"] | undefined,
) {
  return typeof me?.user?.id === "string" && me.user.id.length > 0
    ? JSON.stringify([me.user.id, me.workspace_name, me.authorization])
    : undefined;
}
