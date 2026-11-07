// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import type { components } from "../api/schema";
import { correspondenceSide } from "./messagevoice";

type Activity = components["schemas"]["Activity"];
type Author = components["schemas"]["ActivityAuthor"];
const row: Activity = {
  id: "message",
  kind: "email",
  occurred_at: "2026-09-01T09:00:00Z",
  occurred_precision: "instant",
  is_done: false,
  offers_email: false,
  writable: true,
  source: "manual",
  captured_by: "human:demo",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
};
type WriterCase = {
  direction: Activity["direction"];
  author: Author["kind"] | null;
  side: "ours" | "theirs" | null;
};
const cases: WriterCase[] = JSON.parse(
  readFileSync(
    new URL("./messagevoice.contract.json", import.meta.url),
    "utf8",
  ),
);
it("has the activity engine's generated cases", () =>
  expect(cases.length).toBeGreaterThan(0));
it.each(cases)(
  "reads the activity engine's author side: %j",
  ({ author, direction, side }) => {
    expect(
      correspondenceSide({
        ...row,
        direction,
        ...(author
          ? { voice: { author: { kind: author, is_reader: false } } }
          : {}),
      }),
    ).toBe(side ?? undefined);
  },
);
