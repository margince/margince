// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { bootstrapCopy } from "./bootstrapcopy_gen";
import { en } from "./en";
import { isMessageKey } from "./index";

// census/rules/caption-tables.census.ts finds caption tables the bundle loads.
// The bootstrap captions load before any catalog, so they copy the English one.

describe("tables the shipped bundle loads", () => {
  it("derives every bootstrap error caption from the English catalog", () => {
    for (const [key, value] of Object.entries(bootstrapCopy)) {
      if (!isMessageKey(key))
        throw new Error(`Unknown bootstrap message ${key}`);
      expect(value).toBe(en[key]);
    }
  });
});
