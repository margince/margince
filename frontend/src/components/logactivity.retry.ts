// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef } from "react";
import type { components } from "../api/schema";

type CreateBody = components["schemas"]["CreateActivityRequest"];

type LoggedActivity = Readonly<{
  key: string;
  id: string;
  linkedDealIds: readonly string[];
}>;

// The server answers a key it has stored only for a byte-identical body. The
// body is rebuilt per try, and occurred_at reads the clock, so it is the field to pin.
function sameApartFromStamp(sent: CreateBody, built: CreateBody): boolean {
  const unstamped = (body: CreateBody) =>
    JSON.stringify({ ...body, occurred_at: undefined });
  return unstamped(sent) === unstamped(built);
}

// The first create's answer, kept under its idempotency key, so a retry after a
// later step failed posts no second create. Until an answer arrives, the body
// sent under the key is kept too: a lost answer may hide a stored create.
// An edited draft still goes out as edited: the server releases a refused key.
export function useCreateOncePerKey() {
  const created = useRef<LoggedActivity | null>(null);
  const sent = useRef<{ key: string; body: CreateBody } | null>(null);
  return async (
    key: string,
    built: CreateBody,
    create: (body: CreateBody) => Promise<components["schemas"]["Activity"]>,
  ): Promise<LoggedActivity> => {
    if (created.current?.key === key) return created.current;
    const earlier = sent.current;
    const body =
      earlier?.key === key && sameApartFromStamp(earlier.body, built)
        ? earlier.body
        : built;
    sent.current = { key, body };
    const answer = await create(body);
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
