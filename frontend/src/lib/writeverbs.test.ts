// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { momentVerbWrites } from "./writeverbs";

describe("momentVerbWrites", () => {
  it("answers a known verb from the server's classification", () => {
    expect(momentVerbWrites("log_activity")).toBe(true);
    expect(momentVerbWrites("open_record")).toBe(false);
  });

  it("counts a verb this build does not know as a write", () => {
    expect(momentVerbWrites("a_verb_from_a_newer_server")).toBe(true);
  });
});
