// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef } from "react";
import type { components } from "../api/schema";
import { ProblemError } from "../lib/problem";

type CreateBody = components["schemas"]["CreateActivityRequest"];

type LoggedActivity = Readonly<{
  key: string;
  id: string;
  linkedDealIds: readonly string[];
}>;

// The server answers a key it has stored only for a byte-identical body. An
// instant's occurred_at is read off the clock per try; a chosen day is the draft's.
function sameApartFromStamp(sent: CreateBody, built: CreateBody): boolean {
  const clockStamped =
    sent.occurred_precision === "instant" &&
    built.occurred_precision === "instant";
  const unstamped = (body: CreateBody) =>
    JSON.stringify(clockStamped ? { ...body, occurred_at: undefined } : body);
  return unstamped(sent) === unstamped(built);
}

// The first create's answer, kept under its idempotency key, so a retry after a
// later step failed posts no second create. A body whose answer was lost is kept
// too, since the server may have stored it. An edited draft still goes out as edited.
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
    let answer: components["schemas"]["Activity"];
    try {
      answer = await create(body);
    } catch (error) {
      // An answered refusal stored nothing and releases the key.
      if (error instanceof ProblemError) sent.current = null;
      throw error;
    }
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
