// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { DEAL_STATUS_VALUES, dealStatusTone } from "./dealstatus";

describe("a deal status's tone", () => {
  it("reads won as success, lost as danger, and an open deal untoned", () => {
    expect(
      Object.fromEntries(
        DEAL_STATUS_VALUES.map((status) => [status, dealStatusTone(status)]),
      ),
    ).toEqual({ open: undefined, won: "success", lost: "danger" });
  });

  it("leaves a status this build has no word for untoned", () => {
    expect(dealStatusTone("paused")).toBeUndefined();
  });
});
