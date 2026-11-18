// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef } from "react";
import type { components } from "../api/schema";

type LoggedActivity = Readonly<{
  key: string;
  id: string;
  linkedDealIds: readonly string[];
}>;

// The first create's answer, kept under its idempotency key. A retry after a
// later step failed then posts no second create: the server refuses one whose
// occurred_at has moved on.
export function useCreateOncePerKey() {
  const created = useRef<LoggedActivity | null>(null);
  return async (
    key: string,
    create: () => Promise<components["schemas"]["Activity"]>,
  ): Promise<LoggedActivity> => {
    if (created.current?.key === key) return created.current;
    const answer = await create();
    created.current = {
      key,
      id: answer.id,
      linkedDealIds: (answer.links ?? [])
        .filter((link) => link.entity_type === "deal")
        .map((link) => link.entity_id),
    };
    return created.current;
  };
}
