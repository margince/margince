// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { openValue } from "./openvalue";

describe("the value of a set of open deals", () => {
  it("adds deals in one currency", () => {
    expect(
      openValue(
        [
          { amount_minor: 100, currency: "EUR" },
          { amount_minor: 250, currency: "EUR" },
        ],
        true,
      ),
    ).toEqual({ kind: "sum", minor: 350, currency: "EUR" });
  });

  it("never adds unlike money", () => {
    expect(
      openValue(
        [
          { amount_minor: 100, currency: "EUR" },
          { amount_minor: 100, currency: "USD" },
        ],
        true,
      ).kind,
    ).toBe("mixed");
  });

  it("states no sum when one deal has no readable amount", () => {
    expect(
      openValue(
        [
          { amount_minor: 100, currency: "EUR" },
          { amount_minor: null, currency: "EUR" },
        ],
        true,
      ).kind,
    ).toBe("unknown");
  });

  it("states no sum for a set not read in full, or an empty one", () => {
    expect(openValue([{ amount_minor: 1, currency: "EUR" }], false).kind).toBe(
      "unknown",
    );
    expect(openValue([], true).kind).toBe("unknown");
  });
});
