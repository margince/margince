// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import { unwrap } from "./problem";

export async function readMemberPage(
  cursor: string | null,
  everyone: boolean,
  signal?: AbortSignal,
) {
  const page = unwrap(
    await api.GET("/users", {
      signal,
      params: {
        query: {
          limit: 200,
          ...(everyone ? { include_inactive: true } : {}),
          ...(cursor ? { cursor } : {}),
        },
      },
    }),
  );
  if (page.page.has_more && !page.page.next_cursor)
    throw new Error("The member list is incomplete. Try again.");
  return {
    entries: page.data,
    next: page.page.next_cursor ?? null,
  };
}
