// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { readingsDay } from "../testing/brief.fixtures";
import { listIsComplete } from "./worklistcounts";

const whole = { ...readingsDay({}, []), counts: [], sources_unavailable: [] };

it("calls a page whole only when nothing was cut, failed or held back", () => {
  expect(listIsComplete(whole)).toBe(true);
  expect(listIsComplete({ ...whole, scope_truncated: true })).toBe(false);
  expect(
    listIsComplete({
      ...whole,
      sources_unavailable: [{ source: "task", reason: "timeout" }],
    } as typeof whole),
  ).toBe(false);
  expect(
    listIsComplete({
      ...whole,
      counts: [
        { category: "tasks", considered: 9, shown: 4, more_available: true },
      ],
    } as typeof whole),
  ).toBe(false);
  expect(listIsComplete(undefined)).toBe(false);
});
