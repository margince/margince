// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { ASSIGNED_COUNTRIES, countryOptions } from "./assignedcountries";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the country filter's options", () => {
  it("name every assigned code in the reader's language", () => {
    const options = countryOptions("de");
    expect(options).toHaveLength(ASSIGNED_COUNTRIES.length);
    expect(options.find((option) => option.value === "DE")?.label).toBe(
      "Deutschland",
    );
  });

  it("are built once per language, however often the list renders", () => {
    const first = countryOptions("vi");
    const made = vi.spyOn(Intl, "DisplayNames");
    expect(countryOptions("vi")).toBe(first);
    expect(made).not.toHaveBeenCalled();
    expect(countryOptions("en")).not.toBe(first);
  });
});
